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
type Bus struct {
	cart   *Cartridge
	ppu    *PPU
	apu    *APU
	timer  *Timer
	joypad *Joypad
	serial *Serial

	boot        []byte
	bootEnabled bool

	wram [0x2000]byte
	hram [0x7F]byte
	ie   byte
	ifl  byte

	cycles uint64 // T-cycles elapsed since power on
}

func (b *Bus) requestInterrupt(i byte) { b.ifl |= i }

func (b *Bus) pendingInterrupts() byte { return b.ie & b.ifl & 0x1F }

// tick advances every component by one M-cycle (4 T-cycles).
func (b *Bus) tick() {
	b.timer.tick()
	b.serial.tick()
	b.ppu.tick()
	b.apu.tick()
	b.cycles += 4
}

func (b *Bus) read(addr uint16) byte {
	switch {
	case addr < 0x8000:
		if b.bootEnabled && addr < 0x100 {
			return b.boot[addr]
		}
		return b.cart.readROM(addr)
	case addr < 0xA000:
		return b.ppu.vram[addr-0x8000]
	case addr < 0xC000:
		return b.cart.readRAM(addr)
	case addr < 0xE000:
		return b.wram[addr-0xC000]
	case addr < 0xFE00:
		return b.wram[addr-0xE000]
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
	}
	return 0xFF
}

func (b *Bus) write(addr uint16, v byte) {
	switch {
	case addr < 0x8000:
		b.cart.writeROM(addr, v)
	case addr < 0xA000:
		b.ppu.vram[addr-0x8000] = v
	case addr < 0xC000:
		b.cart.writeRAM(addr, v)
	case addr < 0xE000:
		b.wram[addr-0xC000] = v
	case addr < 0xFE00:
		b.wram[addr-0xE000] = v
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
		if v != 0 {
			b.bootEnabled = false
		}
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
