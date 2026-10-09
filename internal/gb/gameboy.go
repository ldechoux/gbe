package gb

import "errors"

// CyclesPerFrame is the number of T-cycles in one LCD frame (~59.73 Hz).
const CyclesPerFrame = 70224

// Model selects the emulated hardware.
type Model int

const (
	ModelAuto Model = iota // CGB for games that support it, DMG otherwise
	ModelDMG
	ModelCGB // DMG games run colorized, in the compatibility mode
)

// Boot ROM sizes. The CGB one is mapped over 0x0000-0x00FF and
// 0x0200-0x08FF, leaving the cartridge header visible.
const (
	dmgBootSize = 0x100
	cgbBootSize = 0x900
)

// GameBoy wires all the components of a DMG or a CGB together.
type GameBoy struct {
	CPU    *CPU
	Bus    *Bus
	PPU    *PPU
	APU    *APU
	Timer  *Timer
	Joypad *Joypad
	Serial *Serial
	Cart   *Cartridge

	model  Model // ModelDMG or ModelCGB
	boot   []byte
	rumble float64         // see Rumble
	guess  *rumbleDetector // see GuessRumble, nil while off
	// rumbleParams are the settings of the detector, once rumbleTuned
	// (see SetRumbleParams).
	rumbleParams RumbleParams
	rumbleTuned  bool
}

// New creates a Game Boy running the given cartridge, on a CGB if the game
// supports it. See NewModel.
func New(cart *Cartridge, bootROM []byte) (*GameBoy, error) {
	return NewModel(cart, bootROM, ModelAuto)
}

// NewModel creates a Game Boy of the given model running the cartridge.
// bootROM may be nil, in which case the machine starts in the state the boot
// ROM leaves. Otherwise its size must match the model (256 bytes for a DMG,
// 2304 for a CGB).
func NewModel(cart *Cartridge, bootROM []byte, model Model) (*GameBoy, error) {
	model = ResolveModel(cart, model)
	switch {
	case bootROM == nil:
	case model == ModelDMG && len(bootROM) != dmgBootSize:
		return nil, errors.New("the DMG boot ROM must be exactly 256 bytes")
	case model == ModelCGB && len(bootROM) != cgbBootSize:
		return nil, errors.New("the Game Boy Color boot ROM must be exactly 2304 bytes")
	}
	g := &GameBoy{Cart: cart, model: model, boot: bootROM}
	g.Reset()
	return g, nil
}

// BootROMSize returns the size the boot ROM of a model must have.
func BootROMSize(model Model) int {
	if model == ModelCGB {
		return cgbBootSize
	}
	return dmgBootSize
}

// ResolveModel returns the model New would pick for the cartridge.
func ResolveModel(cart *Cartridge, model Model) Model {
	if model != ModelAuto {
		return model
	}
	if cart.ColorSupported() {
		return ModelCGB
	}
	return ModelDMG
}

// IsCGB reports whether the console runs in Game Boy Color mode.
func (g *GameBoy) IsCGB() bool { return g.model == ModelCGB }

// Reset power-cycles the console, keeping the cartridge RAM.
func (g *GameBoy) Reset() {
	g.Cart.stopMotor()
	g.rumble = 0
	bus := &Bus{cart: g.Cart, cgb: g.model == ModelCGB, hdmaLen: 0xFF}
	g.Bus = bus
	g.CPU = &CPU{bus: bus}
	g.PPU = &PPU{bus: bus}
	g.APU = newAPU(bus)
	if g.guess != nil {
		g.guess.attach()
		g.guess.restart()
	}
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

// skipBoot sets the registers to the values the boot ROM leaves behind.
func (g *GameBoy) skipBoot() {
	c := g.CPU
	switch {
	case g.IsCGB() && !g.Cart.ColorSupported():
		// Compatibility mode, with the palette the boot ROM picks for the
		// title, which it also leaves in B.
		combo, checksum := compatAutoCombo(g.Cart)
		g.Bus.key0, g.Bus.compat = 0x04, true
		g.PPU.opri = 1
		c.setAF(0x1180)
		c.setBC(uint16(checksum) << 8)
		c.setDE(0x0008)
		c.setHL(0x007C)
		g.Timer.counter = 0x1EA0
		g.whitePalettes()
		g.PPU.loadCompatCombo(combo)
	case g.IsCGB():
		// A=0x11 is how games detect a Game Boy Color.
		c.setAF(0x1180)
		c.setBC(0x0000)
		c.setDE(0xFF56)
		c.setHL(0x000D)
		g.Timer.counter = 0x1EA0
		g.whitePalettes()
	default:
		c.setAF(0x01B0)
		c.setBC(0x0013)
		c.setDE(0x00D8)
		c.setHL(0x014D)
		g.Timer.counter = 0xABCC
	}
	c.sp = 0xFFFE
	c.pc = 0x0100
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

// whitePalettes sets every color palette to white, as the CGB boot ROM
// leaves them.
func (g *GameBoy) whitePalettes() {
	for i := 0; i < len(g.PPU.bgPal); i += 2 {
		g.PPU.bgPal[i], g.PPU.bgPal[i+1] = 0xFF, 0x7F
		g.PPU.objPal[i], g.PPU.objPal[i+1] = 0xFF, 0x7F
	}
}

// RunFrame runs the emulation until the next VBlank (or for one frame's
// worth of cycles when the LCD is off).
func (g *GameBoy) RunFrame() {
	start := g.Bus.cycles
	g.PPU.frameReady = false
	g.Bus.frameEnd = start + CyclesPerFrame
	for !g.PPU.frameReady && g.Bus.cycles-start < CyclesPerFrame {
		g.CPU.Step()
	}
	g.Bus.frameEnd = 0
	if g.guess != nil {
		g.guess.frame()
	}
	if g.Cart.rumble {
		g.rumble = g.Cart.motorShare(start, g.Bus.cycles)
	}
}

// Rumble returns the share of the last frame during which the rumble motor
// of the cartridge ran, from 0 to 1: how hard it shook. It is always 0 for
// cartridges without a motor.
func (g *GameBoy) Rumble() float64 { return g.rumble }

// HasMotor reports whether the cartridge has a rumble motor. Games without
// one may still shake through GuessedRumble.
func (g *GameBoy) HasMotor() bool { return g.Cart.rumble }

// Framebuffer returns the last complete frame in DMG mode, one shade (0-3)
// per pixel.
func (g *GameBoy) Framebuffer() *[ScreenWidth * ScreenHeight]byte { return &g.PPU.front }

// ColorFramebuffer returns the last complete frame in CGB mode, one RGB555
// color per pixel (red in the low bits, as stored in the palette RAM).
func (g *GameBoy) ColorFramebuffer() *[ScreenWidth * ScreenHeight]uint16 { return &g.PPU.cfront }

// SetButton updates the state of one button.
func (g *GameBoy) SetButton(b Button, pressed bool) { g.Joypad.set(b, pressed) }

// Cycles returns the T-cycles run since power on, at the normal speed: it
// goes back when a state is loaded.
func (g *GameBoy) Cycles() uint64 { return g.Bus.cycles }

// PC returns the program counter (handy for tests and debugging).
func (g *GameBoy) PC() uint16 { return g.CPU.pc }

// BootROMActive reports whether the boot ROM is still mapped.
func (g *GameBoy) BootROMActive() bool { return g.Bus.bootEnabled }
