package gb

// The Game Boy Camera (Pocket Camera in Japan), cartridge type 0xFC: a
// 1 MiB ROM, 128 KiB of battery-backed RAM, the MAC-GBD controller and a
// Mitsubishi M64282FP sensor. See Pan Docs, "Game Boy Camera", and SameBoy
// (Core/camera.c, Expat license), whose model of the sensor this follows:
// only what the camera ROM uses is emulated.
//
// Bank 0x10 of the RAM area (any bank with bit 4 set) maps the registers
// of the camera at A000, mirrored every 0x80 bytes:
//
//	A000     bit 0 starts a capture, and reads 1 until it is done
//	A001     N (bit 7), VH (bits 6-5), gain (bits 4-0)
//	A002-03  exposure time, big endian
//	A004     edge enhancement ratio (bits 6-4), invert, output voltage
//	A005     zero point and output reference voltage
//	A006-35  dithering: 4x4 thresholds, 3 per pixel, one per shade
//
// A capture writes a 128x112 picture to RAM bank 0, at A100, as 16x14
// tiles of 2 bits per pixel. The game copies it to another bank to keep it.

const (
	// CameraWidth and CameraHeight are the size of a picture of the camera.
	CameraWidth  = 128
	CameraHeight = 112

	cameraRegs  = 0x36
	cameraImage = 0x100 // offset of the picture in RAM bank 0
)

// CameraImage is what the sensor sees: CameraWidth x CameraHeight levels
// of gray, row by row, from 0 (black) to 255 (white).
type CameraImage [CameraWidth * CameraHeight]byte

// CameraSource fills img with what the sensor sees at the time of a
// capture, and reports whether it did: without a picture, the sensor sees
// noise, as a real one in the dark.
type CameraSource func(img *CameraImage) bool

type camera struct {
	c         *Cartridge
	ramEnable bool
	bank      byte // ROM bank, 6 bits
	ramBank   byte // bit 4 maps the registers
	regs      [cameraRegs]byte
	busy      bool
	done      uint64 // T-cycle the capture ends at
}

// IsCamera reports whether the cartridge is a Game Boy Camera.
func (c *Cartridge) IsCamera() bool {
	_, ok := c.mbc.(*camera)
	return ok
}

// SetCameraSource sets what the sensor of a Game Boy Camera sees; nil
// gives it noise. It is called on the thread that runs the console, at
// the end of each capture.
func (c *Cartridge) SetCameraSource(src CameraSource) { c.cameraSrc = src }

// stopCapture drops the capture under way, as when the console is reset:
// its end was counted in the T-cycles of the console before.
func (c *Cartridge) stopCapture() {
	if m, ok := c.mbc.(*camera); ok {
		m.busy = false
		m.regs[0] &^= 1
	}
}

func (m *camera) readROM(addr uint16) byte {
	if addr < 0x4000 {
		return m.c.romByte(0, addr)
	}
	return m.c.romByte(int(m.bank), addr)
}

func (m *camera) writeROM(addr uint16, v byte) {
	switch {
	case addr < 0x2000:
		m.ramEnable = v&0x0F == 0x0A
	case addr < 0x4000:
		m.bank = v & 0x3F
	case addr < 0x6000:
		m.ramBank = v & 0x1F
	}
}

func (m *camera) readRAM(addr uint16) byte {
	m.update()
	if m.ramBank&0x10 != 0 {
		if addr&0x7F == 0 {
			v := m.regs[0] & 0x06
			if m.busy {
				v |= 1
			}
			return v
		}
		return 0
	}
	if m.busy { // the controller holds the RAM while it captures
		return 0
	}
	return m.c.ram[m.c.ramOffset(int(m.ramBank), addr)]
}

func (m *camera) writeRAM(addr uint16, v byte) {
	m.update()
	if m.ramBank&0x10 != 0 {
		m.writeReg(byte(addr&0x7F), v)
		return
	}
	if !m.ramEnable || m.busy {
		return
	}
	m.c.ram[m.c.ramOffset(int(m.ramBank), addr)] = v
	m.c.ramDirty = true
}

func (m *camera) writeReg(r, v byte) {
	switch {
	case r == 0:
		m.regs[0] = v & 0x07
		if v&1 != 0 && !m.busy {
			m.busy = true
			m.done = m.c.now() + m.captureCycles()
		}
	case r < cameraRegs:
		m.regs[r] = v
	}
}

// captureCycles is how long a capture takes, in T-cycles: Pan Docs gives
// 32446 + (N ? 0 : 512) + 16 x exposure, in M-cycles.
func (m *camera) captureCycles() uint64 {
	cycles := 32446 + 16*uint64(m.exposure())
	if m.regs[1]&0x80 == 0 {
		cycles += 512
	}
	return 4 * cycles
}

func (m *camera) exposure() int { return int(m.regs[2])<<8 | int(m.regs[3]) }

// update ends the capture under way once its time has come. The picture is
// taken then, from the registers the game set: it does not touch them
// while the camera works.
func (m *camera) update() {
	if !m.busy || m.c.now() < m.done {
		return
	}
	m.busy = false
	m.regs[0] &^= 1
	var img CameraImage
	if src := m.c.cameraSrc; src == nil || !src(&img) {
		cameraNoise(&img, m.done)
	}
	m.develop(&img, m.c.ram[cameraImage:cameraImage+CameraWidth*CameraHeight/4])
	m.c.ramDirty = true
}

// cameraGains is the factor of each value of the gain (A001, bits 4-0),
// in 1/65536: from 0.881 to 1.667.
var cameraGains = [32]int64{
	57733, 59960, 61981, 63830, 65536, 67118, 68593, 69976,
	71276, 73664, 75818, 77778, 79577, 81240, 83518, 86228,
	88639, 90810, 92785, 94597, 96270, 97824, 99275, 100635,
	101916, 103126, 104273, 105362, 106400, 107391, 108339, 109247,
}

// cameraEdgeRatios is the strength of the edge enhancement (A004, bits
// 6-4), in quarters: from 0.5 to 5.
var cameraEdgeRatios = [8]int64{2, 3, 4, 5, 8, 12, 16, 20}

// develop turns what the sensor sees into the tiles of the picture: the
// light grows with the exposure time (0x1000 for the picture as seen) and
// the gain, the edges are enhanced in the 2D mode the ROM uses, and the
// thresholds of the dithering matrix give each pixel one of the 4 shades.
// It counts in integers, for the same pictures on every machine.
func (m *camera) develop(img *CameraImage, out []byte) {
	// light is in 1/2^28 of a level: the gain in 1/2^16, the exposure 2^12.
	scale := cameraGains[m.regs[1]&0x1F] * int64(m.exposure())
	var light [CameraWidth * CameraHeight]int64
	for i, v := range img {
		light[i] = int64(v) * scale
	}
	at := func(x, y int) int64 {
		x = min(max(x, 0), CameraWidth-1)
		y = min(max(y, 0), CameraHeight-1)
		return light[y*CameraWidth+x]
	}
	edges := m.regs[1]&0xE0 == 0xE0
	ratio := cameraEdgeRatios[m.regs[4]>>4&7]
	clear(out)
	for y := range CameraHeight {
		for x := range CameraWidth {
			v := 4 * at(x, y) // in quarters, as the ratio
			if edges {
				v += ratio * (v - at(x-1, y) - at(x+1, y) - at(x, y-1) - at(x, y+1))
			}
			t := m.regs[6+((x&3)+(y&3)*4)*3:]
			shade := 0
			switch {
			case v < int64(t[0])<<30:
				shade = 3
			case v < int64(t[1])<<30:
				shade = 2
			case v < int64(t[2])<<30:
				shade = 1
			}
			i := (y/8*16+x/8)*16 + y%8*2 // tile row, low then high bit plane
			bit := byte(0x80) >> (x % 8)
			if shade&1 != 0 {
				out[i] |= bit
			}
			if shade&2 != 0 {
				out[i+1] |= bit
			}
		}
	}
}

// cameraNoise fills img with the noise of a sensor that sees nothing, from
// seed so that the same capture gives the same picture.
func cameraNoise(img *CameraImage, seed uint64) {
	for i := range img { // splitmix64
		seed += 0x9E3779B97F4A7C15
		z := seed
		z = (z ^ z>>30) * 0xBF58476D1CE4E5B9
		z = (z ^ z>>27) * 0x94D049BB133111EB
		img[i] = byte((z ^ z>>31) >> 56)
	}
}

func (m *camera) sync(c *codec) {
	c.bool(&m.ramEnable)
	c.u8(&m.bank)
	c.u8(&m.ramBank)
	c.raw(m.regs[:])
	c.bool(&m.busy)
	c.u64(&m.done)
}
