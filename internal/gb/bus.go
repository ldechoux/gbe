package gb

// Interrupt bits, as found in IE (0xFFFF) and IF (0xFF0F).
const (
	IntVBlank byte = 1 << iota
	IntSTAT
	IntTimer
	IntSerial
	IntJoypad
)

// Bus routes CPU memory accesses to the various components and advances
// them in lockstep with the CPU (every access costs one M-cycle).
//
// In CGB mode it also holds the second half of the hardware: the banked
// WRAM, the speed switch and the VRAM DMA.
type Bus struct {
	cart   *Cartridge
	ppu    *PPU
	apu    *APU
	timer  *Timer
	joypad *Joypad
	serial *Serial

	cgb         bool
	boot        []byte
	bootEnabled bool

	// A DMG game on a CGB: the boot ROM writes 0x04 to KEY0, and once it
	// is unmapped the console runs in compatibility mode, where the CGB
	// registers are gone and the DMG palettes index the color ones.
	key0   byte
	compat bool

	wram [0x8000]byte // 8 banks of 4 KiB; a DMG only uses the first two
	svbk byte         // WRAM bank mapped at 0xD000 (CGB)
	hram [0x7F]byte
	ie   byte
	ifl  byte

	doubleSpeed bool // CGB double speed mode
	speedArmed  bool // KEY1 bit 0: STOP switches the speed

	// VRAM DMA (CGB). hdmaLen is the number of 16-byte blocks left minus
	// one, 0xFF when no transfer is pending.
	hdmaSrc, hdmaDst uint16
	hdmaLen          byte
	hdmaActive       bool // HBlank transfer in progress
	stall            int  // M-cycles the CPU waits for a VRAM DMA

	cycles uint64 // T-cycles elapsed since power on, at the normal speed
}

func (b *Bus) requestInterrupt(i byte) { b.ifl |= i }

// cgbMode reports whether the CGB features are available: on a CGB, except
// in compatibility mode.
func (b *Bus) cgbMode() bool { return b.cgb && !b.compat }

func (b *Bus) pendingInterrupts() byte { return b.ie & b.ifl & 0x1F }

// tick advances every component by one M-cycle. That is 4 T-cycles, or 2
// in double speed mode: the timer and the serial port follow the CPU clock,
// but the PPU and the APU keep running at the normal speed.
func (b *Bus) tick() {
	b.timer.tick()
	b.serial.tick()
	dots := 4
	if b.doubleSpeed {
		dots = 2
	}
	b.ppu.tick(dots)
	b.apu.tick(dots)
	b.cycles += uint64(dots)
}

// wramOffset maps 0xC000-0xDFFF (or its echo) to the WRAM array.
func (b *Bus) wramOffset(addr uint16) int {
	addr &= 0x1FFF
	if addr < 0x1000 {
		return int(addr)
	}
	bank := int(b.svbk & 7)
	if bank == 0 {
		bank = 1
	}
	return bank*0x1000 + int(addr-0x1000)
}

func (b *Bus) bootMapped(addr uint16) bool {
	return b.bootEnabled && (addr < 0x100 || len(b.boot) == cgbBootSize && addr >= 0x200 && int(addr) < cgbBootSize)
}

func (b *Bus) read(addr uint16) byte {
	switch {
	case addr < 0x8000:
		if b.bootMapped(addr) {
			return b.boot[addr]
		}
		return b.cart.readROM(addr)
	case addr < 0xA000:
		return b.ppu.vram[b.ppu.vramOffset(addr)]
	case addr < 0xC000:
		return b.cart.readRAM(addr)
	case addr < 0xFE00:
		return b.wram[b.wramOffset(addr)]
	case addr < 0xFEA0:
		return b.ppu.oam[addr-0xFE00]
	case addr < 0xFF00:
		return 0xFF
	case addr >= 0xFF80 && addr < 0xFFFF:
		return b.hram[addr-0xFF80]
	case addr == 0xFFFF:
		return b.ie
	}
	return b.readIO(addr)
}

func (b *Bus) readIO(addr uint16) byte {
	switch {
	case addr == 0xFF00:
		return b.joypad.read()
	case addr == 0xFF01 || addr == 0xFF02:
		return b.serial.read(addr)
	case addr >= 0xFF04 && addr <= 0xFF07:
		return b.timer.read(addr)
	case addr == 0xFF0F:
		return b.ifl | 0xE0
	case addr >= 0xFF10 && addr <= 0xFF3F:
		return b.apu.read(addr)
	case addr >= 0xFF40 && addr <= 0xFF4B:
		return b.ppu.read(addr)
	case !b.cgbMode():
		return 0xFF
	case addr == 0xFF4D:
		v := byte(0x7E)
		if b.doubleSpeed {
			v |= 0x80
		}
		if b.speedArmed {
			v |= 0x01
		}
		return v
	case addr == 0xFF4F || addr >= 0xFF68 && addr <= 0xFF6C:
		return b.ppu.read(addr)
	case addr == 0xFF55:
		if b.hdmaActive {
			return b.hdmaLen & 0x7F
		}
		return 0x80 | b.hdmaLen
	case addr == 0xFF70:
		return 0xF8 | b.svbk
	}
	return 0xFF
}

func (b *Bus) write(addr uint16, v byte) {
	switch {
	case addr < 0x8000:
		b.cart.writeROM(addr, v)
	case addr < 0xA000:
		b.ppu.vram[b.ppu.vramOffset(addr)] = v
	case addr < 0xC000:
		b.cart.writeRAM(addr, v)
	case addr < 0xFE00:
		b.wram[b.wramOffset(addr)] = v
	case addr < 0xFEA0:
		b.ppu.oam[addr-0xFE00] = v
	case addr < 0xFF00:
		// unusable area
	case addr >= 0xFF80 && addr < 0xFFFF:
		b.hram[addr-0xFF80] = v
	case addr == 0xFFFF:
		b.ie = v
	default:
		b.writeIO(addr, v)
	}
}

func (b *Bus) writeIO(addr uint16, v byte) {
	switch {
	case addr == 0xFF00:
		b.joypad.write(v)
	case addr == 0xFF01 || addr == 0xFF02:
		b.serial.write(addr, v)
	case addr >= 0xFF04 && addr <= 0xFF07:
		b.timer.write(addr, v)
	case addr == 0xFF0F:
		b.ifl = v & 0x1F
	case addr >= 0xFF10 && addr <= 0xFF3F:
		b.apu.write(addr, v)
	case addr == 0xFF46:
		b.ppu.dmaReg = v
		b.oamDMA(v)
	case addr >= 0xFF40 && addr <= 0xFF4B:
		b.ppu.write(addr, v)
	case addr == 0xFF50:
		if v != 0 && b.bootEnabled {
			b.bootEnabled = false
			b.compat = b.cgb && b.key0&0x0C != 0
		}
	case addr == 0xFF4C:
		if b.cgb && b.bootEnabled { // locked once the game starts
			b.key0 = v
		}
	case !b.cgbMode():
	case addr == 0xFF4D:
		b.speedArmed = v&1 != 0
	case addr == 0xFF4F || addr >= 0xFF68 && addr <= 0xFF6C:
		b.ppu.write(addr, v)
	case addr == 0xFF51:
		b.hdmaSrc = b.hdmaSrc&0x00FF | uint16(v)<<8
	case addr == 0xFF52:
		b.hdmaSrc = b.hdmaSrc&0xFF00 | uint16(v&0xF0)
	case addr == 0xFF53:
		b.hdmaDst = b.hdmaDst&0x00FF | uint16(v&0x1F)<<8
	case addr == 0xFF54:
		b.hdmaDst = b.hdmaDst&0xFF00 | uint16(v&0xF0)
	case addr == 0xFF55:
		b.startVRAMDMA(v)
	case addr == 0xFF70:
		b.svbk = v & 7
	}
}

// switchSpeed is run by STOP: when armed through KEY1, it toggles the CGB
// double speed mode. It reports whether the speed changed.
func (b *Bus) switchSpeed() bool {
	if !b.cgbMode() || !b.speedArmed {
		return false
	}
	b.speedArmed = false
	b.doubleSpeed = !b.doubleSpeed
	return true
}

// startVRAMDMA handles a write to HDMA5 (0xFF55).
func (b *Bus) startVRAMDMA(v byte) {
	if b.hdmaActive {
		if v&0x80 == 0 { // cancel the HBlank transfer
			b.hdmaActive = false
			return
		}
	}
	b.hdmaLen = v & 0x7F
	if v&0x80 == 0 {
		// General purpose DMA: everything now, the CPU waits.
		for !b.hdmaDone() {
			b.vramDMABlock()
		}
		return
	}
	b.hdmaActive = true
	if b.ppu.lcdc&0x80 == 0 || b.ppu.mode == 0 {
		b.vramDMABlock() // started during HBlank (or LCD off): one block at once
	}
}

func (b *Bus) hdmaDone() bool { return b.hdmaLen == 0xFF }

// hblank is called by the PPU at the start of each HBlank.
func (b *Bus) hblank() {
	if b.hdmaActive {
		b.vramDMABlock()
	}
}

// vramDMABlock copies 16 bytes to VRAM. It takes the CPU 8 M-cycles at the
// normal speed and 16 in double speed mode, the copy being clocked by the
// PPU.
func (b *Bus) vramDMABlock() {
	for i := range uint16(16) {
		v := b.read(b.hdmaSrc + i)
		b.ppu.vram[b.ppu.vramOffset(0x8000|(b.hdmaDst+i)&0x1FFF)] = v
	}
	b.hdmaSrc += 16
	b.hdmaDst = (b.hdmaDst + 16) & 0x1FFF
	b.hdmaLen--
	if b.hdmaDone() {
		b.hdmaActive = false
	}
	if b.doubleSpeed {
		b.stall += 16
	} else {
		b.stall += 8
	}
}

// oamDMA copies 160 bytes from v<<8 into OAM. The transfer is performed
// instantly, which is transparent for games that wait in HRAM as intended.
func (b *Bus) oamDMA(v byte) {
	src := uint16(v) << 8
	if src >= 0xE000 {
		src -= 0x2000
	}
	for i := uint16(0); i < 0xA0; i++ {
		b.ppu.oam[i] = b.read(src + i)
	}
}
