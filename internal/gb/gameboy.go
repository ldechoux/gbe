package gb

import "errors"

// CyclesPerFrame is the number of T-cycles in one LCD frame (~59.73 Hz).
const CyclesPerFrame = 70224

// GameBoy wires all the components of a DMG together.
type GameBoy struct {
	CPU    *CPU
	Bus    *Bus
	PPU    *PPU
	APU    *APU
	Timer  *Timer
	Joypad *Joypad
	Serial *Serial
	Cart   *Cartridge

	boot []byte
}

// New creates a Game Boy running the given cartridge. bootROM may be nil,
// in which case the machine starts in the state the DMG boot ROM leaves.
func New(cart *Cartridge, bootROM []byte) (*GameBoy, error) {
	if bootROM != nil && len(bootROM) != 0x100 {
		return nil, errors.New("boot ROM must be exactly 256 bytes")
	}
	g := &GameBoy{Cart: cart, boot: bootROM}
	g.Reset()
	return g, nil
}

// Reset power-cycles the console, keeping the cartridge RAM.
func (g *GameBoy) Reset() {
	bus := &Bus{cart: g.Cart}
	g.Bus = bus
	g.CPU = &CPU{bus: bus}
	g.PPU = &PPU{bus: bus}
	g.APU = newAPU(bus)
	g.Timer = &Timer{bus: bus}
	g.Joypad = &Joypad{bus: bus, sel: 0x30}
	g.Serial = &Serial{bus: bus}
	bus.ppu, bus.apu, bus.timer, bus.joypad, bus.serial = g.PPU, g.APU, g.Timer, g.Joypad, g.Serial

	if g.boot != nil {
		bus.boot = g.boot
		bus.bootEnabled = true
		return
	}
	g.skipBoot()
}

// skipBoot sets the registers to the values the DMG boot ROM leaves behind.
func (g *GameBoy) skipBoot() {
	c := g.CPU
	c.setAF(0x01B0)
	c.setBC(0x0013)
	c.setDE(0x00D8)
	c.setHL(0x014D)
	c.sp = 0xFFFE
	c.pc = 0x0100
	g.Timer.counter = 0xABCC
	io := []struct {
		addr uint16
		v    byte
	}{
		// NR52 first so the APU accepts the other writes. Trigger bits of
		// NRx4 are left out so no channel starts playing.
		{0xFF26, 0x80}, {0xFF10, 0x80}, {0xFF11, 0xBF}, {0xFF12, 0xF3}, {0xFF14, 0x3F},
		{0xFF16, 0x3F}, {0xFF17, 0x00}, {0xFF19, 0x3F}, {0xFF1A, 0x7F}, {0xFF1B, 0xFF},
		{0xFF1C, 0x9F}, {0xFF1E, 0x3F}, {0xFF20, 0xFF}, {0xFF21, 0x00}, {0xFF22, 0x00},
		{0xFF23, 0x3F}, {0xFF24, 0x77}, {0xFF25, 0xF3},
		{0xFF40, 0x91}, {0xFF47, 0xFC}, {0xFF48, 0xFF}, {0xFF49, 0xFF},
	}
	for _, r := range io {
		g.Bus.write(r.addr, r.v)
	}
	g.Bus.ifl = 0x01
}

// RunFrame runs the emulation until the next VBlank (or for one frame's
// worth of cycles when the LCD is off).
func (g *GameBoy) RunFrame() {
	start := g.Bus.cycles
	g.PPU.frameReady = false
	for !g.PPU.frameReady && g.Bus.cycles-start < CyclesPerFrame {
		g.CPU.Step()
	}
}

// Framebuffer returns the last complete frame, one shade (0-3) per pixel.
func (g *GameBoy) Framebuffer() *[ScreenWidth * ScreenHeight]byte { return &g.PPU.front }

// SetButton updates the state of one button.
func (g *GameBoy) SetButton(b Button, pressed bool) { g.Joypad.set(b, pressed) }

// PC returns the program counter (handy for tests and debugging).
func (g *GameBoy) PC() uint16 { return g.CPU.pc }

// BootROMActive reports whether the boot ROM is still mapped.
func (g *GameBoy) BootROMActive() bool { return g.Bus.bootEnabled }
