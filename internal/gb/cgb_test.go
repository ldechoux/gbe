package gb

import "testing"

// newTestCGB builds a Game Boy Color around a 32 KiB ROM-only cartridge
// flagged as CGB compatible, whose code starts at 0x0100.
func newTestCGB(t *testing.T, program ...byte) *GameBoy {
	t.Helper()
	rom := make([]byte, 0x8000)
	rom[0x143] = 0x80
	copy(rom[0x100:], program)
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	g, err := New(cart, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !g.IsCGB() {
		t.Fatal("CGB compatible cartridge not run in CGB mode")
	}
	g.Bus.ifl = 0
	return g
}

func TestCGBHeader(t *testing.T) {
	rom := make([]byte, 0x8000)
	copy(rom[0x134:], "POKEMON CRYSTAL")
	rom[0x143] = 0xC0
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	if cart.Title != "POKEMON CRYSTAL" || !cart.ColorSupported() || !cart.ColorOnly() {
		t.Errorf("title %q, CGB %02X", cart.Title, cart.CGB)
	}
	if _, err := New(cart, make([]byte, 0x100)); err == nil {
		t.Error("DMG boot ROM accepted for a Game Boy Color game")
	}
	if _, err := New(cart, make([]byte, 0x900)); err != nil {
		t.Errorf("CGB boot ROM refused: %v", err)
	}
	g, err := NewModel(cart, nil, ModelDMG)
	if err != nil || g.IsCGB() {
		t.Errorf("forcing the DMG model: %v", err)
	}

	rom[0x143] = 0x00
	dmg, _ := NewCartridge(rom)
	if _, err := NewModel(dmg, nil, ModelCGB); err == nil {
		t.Error("DMG only game accepted in CGB mode")
	}
	if g, _ := New(dmg, nil); g.IsCGB() {
		t.Error("DMG only game run in CGB mode")
	}
}

func TestCGBBootState(t *testing.T) {
	g := newTestCGB(t)
	if g.CPU.a != 0x11 || g.CPU.f != 0x80 {
		t.Errorf("A=%02X F=%02X, want 11 80", g.CPU.a, g.CPU.f)
	}
	if c := color(&g.PPU.bgPal, 7, 3); c != 0x7FFF {
		t.Errorf("BG palette 7 color 3 = %04X, want white", c)
	}
}

func TestCGBBootOverlay(t *testing.T) {
	g := newTestCGB(t)
	boot := make([]byte, 0x900)
	boot[0x0000], boot[0x0150], boot[0x0200], boot[0x08FF] = 0x31, 0x99, 0x42, 0x43
	g, err := New(g.Cart, boot)
	if err != nil {
		t.Fatal(err)
	}
	g.Cart.rom[0x0150] = 0x77
	if g.Bus.read(0x0000) != 0x31 || g.Bus.read(0x0200) != 0x42 || g.Bus.read(0x08FF) != 0x43 {
		t.Error("CGB boot ROM not mapped over 0x0000-0x00FF and 0x0200-0x08FF")
	}
	if g.Bus.read(0x0150) != 0x77 {
		t.Error("the cartridge header is hidden by the boot ROM")
	}
}

func TestWRAMBanks(t *testing.T) {
	g := newTestCGB(t)
	b := g.Bus
	if got := b.read(0xFF70); got != 0xF8 {
		t.Errorf("SVBK %02X, want F8", got)
	}
	b.write(0xC000, 0x10)
	for bank := byte(1); bank < 8; bank++ {
		b.write(0xFF70, bank)
		b.write(0xD000, bank)
	}
	b.write(0xFF70, 0) // bank 0 selects bank 1
	if got := b.read(0xD000); got != 1 {
		t.Errorf("SVBK=0 reads bank %d, want 1", got)
	}
	for bank := byte(1); bank < 8; bank++ {
		b.write(0xFF70, bank)
		if got := b.read(0xD000); got != bank {
			t.Errorf("bank %d reads %d", bank, got)
		}
		if got := b.read(0xF000); got != bank {
			t.Errorf("echo of bank %d reads %d", bank, got)
		}
		if got := b.read(0xC000); got != 0x10 {
			t.Errorf("bank 0 changed with SVBK=%d", bank)
		}
	}
}

func TestVRAMBanks(t *testing.T) {
	g := newTestCGB(t)
	b := g.Bus
	b.write(0x8000, 0xAA)
	b.write(0xFF4F, 0xFF)
	if got := b.read(0xFF4F); got != 0xFF {
		t.Errorf("VBK %02X, want FF", got)
	}
	if b.read(0x8000) != 0x00 {
		t.Error("bank 1 shows bank 0 data")
	}
	b.write(0x9FFF, 0xBB)
	b.write(0xFF4F, 0)
	if b.read(0x8000) != 0xAA || b.read(0x9FFF) != 0x00 || g.PPU.vram[0x3FFF] != 0xBB {
		t.Error("VRAM banks are not separate")
	}
}

func TestDMGIgnoresCGBRegisters(t *testing.T) {
	g := newTestGB(t)
	b := g.Bus
	for _, r := range []uint16{0xFF4D, 0xFF4F, 0xFF55, 0xFF68, 0xFF69, 0xFF6A, 0xFF6B, 0xFF6C, 0xFF70} {
		b.write(r, 0x01)
		if got := b.read(r); got != 0xFF {
			t.Errorf("register %04X reads %02X on a DMG, want FF", r, got)
		}
	}
	b.write(0xD000, 0x55)
	if b.read(0xD000) != 0x55 || b.wram[0x1000] != 0x55 {
		t.Error("DMG WRAM bank 1 moved")
	}
}

func TestColorPalettes(t *testing.T) {
	g := newTestCGB(t)
	b := g.Bus
	b.write(0xFF68, 0x80|0x3F) // auto-increment from the last byte
	b.write(0xFF69, 0x1F)
	b.write(0xFF69, 0x7C) // wraps to index 0
	if got := b.read(0xFF68); got != 0xC1 {
		t.Errorf("BCPS %02X, want C1", got)
	}
	if g.PPU.bgPal[0x3F] != 0x1F || g.PPU.bgPal[0] != 0x7C {
		t.Error("BCPD writes not stored at the index")
	}
	b.write(0xFF68, 0x3F)
	if got := b.read(0xFF69); got != 0x1F {
		t.Errorf("BCPD reads %02X, want 1F", got)
	}
	b.write(0xFF69, 0x20) // no auto-increment
	if got := b.read(0xFF68); got != 0x7F {
		t.Errorf("BCPS %02X after a write without auto-increment, want 7F", got)
	}

	b.write(0xFF6A, 0x80|0x08) // OBJ palette 1, color 0
	b.write(0xFF6B, 0xE0)
	b.write(0xFF6B, 0x03)
	if c := color(&g.PPU.objPal, 1, 0); c != 0x03E0 {
		t.Errorf("OBJ palette 1 color 0 = %04X, want 03E0 (green)", c)
	}
}

func TestGeneralDMA(t *testing.T) {
	g := newTestCGB(t)
	b := g.Bus
	for i := range 0x40 {
		b.write(0xC100+uint16(i), byte(i+1))
	}
	b.write(0xFF4F, 1)
	b.write(0xFF51, 0xC1)
	b.write(0xFF52, 0x0F) // low 4 bits ignored
	b.write(0xFF53, 0xE8) // top 3 bits ignored: 0x8800
	b.write(0xFF54, 0x00)
	b.write(0xFF55, 0x03) // 4 blocks
	for i := range 0x40 {
		if got := g.PPU.vram[0x2800+i]; got != byte(i+1) {
			t.Fatalf("VRAM bank 1 0x%04X = %02X, want %02X", 0x8800+i, got, i+1)
		}
	}
	if got := b.read(0xFF55); got != 0xFF {
		t.Errorf("HDMA5 %02X after the transfer, want FF", got)
	}
	if b.stall != 4*8 {
		t.Errorf("CPU stalled %d M-cycles, want 32", b.stall)
	}
}

func TestHBlankDMA(t *testing.T) {
	g := newTestCGB(t)
	b := g.Bus
	for i := range 0x30 {
		b.write(0xC000+uint16(i), byte(0x80+i))
	}
	// Wait for the start of a line (mode 2) so no block is copied at once.
	for g.PPU.mode != 2 {
		b.tick()
	}
	b.write(0xFF51, 0xC0)
	b.write(0xFF52, 0x00)
	b.write(0xFF53, 0x00)
	b.write(0xFF54, 0x00)
	b.write(0xFF55, 0x82) // 3 blocks, during HBlank
	if got := b.read(0xFF55); got != 0x02 {
		t.Errorf("HDMA5 %02X, want 02 (active, 3 blocks left)", got)
	}
	for g.PPU.mode != 0 {
		b.tick()
	}
	if g.PPU.vram[0x0F] != 0x8F || g.PPU.vram[0x10] != 0 {
		t.Error("first HBlank did not copy exactly one block")
	}
	if got := b.read(0xFF55); got != 0x01 {
		t.Errorf("HDMA5 %02X after one block, want 01", got)
	}

	b.write(0xFF55, 0x00) // cancel
	if got := b.read(0xFF55); got != 0x81 {
		t.Errorf("HDMA5 %02X after cancelling, want 81", got)
	}
	for range 456 { // a whole line
		b.tick()
	}
	if g.PPU.vram[0x10] != 0 {
		t.Error("cancelled HBlank DMA kept copying")
	}
}

func TestSpeedSwitch(t *testing.T) {
	// STOP with KEY1 armed toggles the double speed mode.
	g := newTestCGB(t, 0x10, 0x00, 0x00)
	b := g.Bus
	if got := b.read(0xFF4D); got != 0x7E {
		t.Errorf("KEY1 %02X, want 7E", got)
	}
	b.write(0xFF4D, 0x01)
	if got := b.read(0xFF4D); got != 0x7F {
		t.Errorf("KEY1 %02X once armed, want 7F", got)
	}
	g.CPU.Step()
	if !b.doubleSpeed || b.read(0xFF4D) != 0xFE {
		t.Fatalf("KEY1 %02X after STOP, want FE", b.read(0xFF4D))
	}
	if b.stall == 0 {
		t.Error("the CPU does not wait for the clock to settle")
	}

	// In double speed the PPU and the APU get half as many dots per M-cycle.
	dot, cycles := g.PPU.dot, b.cycles
	b.tick()
	if g.PPU.dot-dot != 2 || b.cycles-cycles != 2 {
		t.Errorf("one M-cycle advanced the PPU by %d dots, want 2", g.PPU.dot-dot)
	}
	// The timer follows the CPU clock.
	counter := g.Timer.counter
	b.tick()
	if g.Timer.counter-counter != 4 {
		t.Errorf("DIV counter advanced by %d per M-cycle, want 4", g.Timer.counter-counter)
	}

	// STOP without arming KEY1 keeps the speed.
	g.CPU.pc = 0x0100
	b.stall = 0
	g.CPU.Step()
	if !b.doubleSpeed {
		t.Error("STOP switched the speed without KEY1 armed")
	}
}

func TestCGBSpritePriority(t *testing.T) {
	g := newTestCGB(t)
	p := g.PPU
	p.bgPal, p.objPal = [64]byte{}, [64]byte{}
	// Tile 1: solid color 1. Tile 2: solid color 2.
	for i := range 8 {
		p.vram[16+i*2] = 0xFF
		p.vram[32+i*2+1] = 0xFF
	}
	// Sprite 0 covers X=12-19 with tile 2, sprite 1 covers X=8-15 with
	// tile 1 (lower X).
	copy(p.oam[:], []byte{16, 20, 2, 0x01, 16, 16, 1, 0x02})
	p.objPal[1*8+2*2] = 0x1F   // palette 1, color 2: red
	p.objPal[2*8+1*2+1] = 0x7C // palette 2, color 1: blue
	p.lcdc = 0x83
	p.ly = 0
	p.renderLine()
	// A CGB draws the lower OAM index on top, wherever it is.
	if got := p.cback[12]; got != 0x001F {
		t.Errorf("overlap pixel %04X, want sprite 0 (001F)", got)
	}
	if got := p.cback[8]; got != 0x7C00 {
		t.Errorf("pixel 8 %04X, want sprite 1 (7C00)", got)
	}

	// With OPRI bit 0 set, priority goes back to the DMG rule (lower X).
	g.Bus.write(0xFF6C, 1)
	p.renderLine()
	if got := p.cback[12]; got != 0x7C00 {
		t.Errorf("overlap pixel %04X with OPRI=1, want sprite 1 (7C00)", got)
	}
}

func TestCGBBackgroundAttributes(t *testing.T) {
	g := newTestCGB(t)
	p := g.PPU
	p.bgPal = [64]byte{}
	// Tile 0 in bank 1: row 0 has color 1 on its leftmost pixel only.
	p.vram[0x2000] = 0x80
	// Map entry 0 uses bank 1, palette 3 and X flip; entry 1 is plain tile 0.
	p.vram[0x2000+0x1800] = 0x08 | 0x03 | 0x20
	p.bgPal[3*8+1*2] = 0x1F   // palette 3 color 1: red
	p.bgPal[0*8+0*2] = 0xE0   // palette 0 color 0: some green
	p.bgPal[0*8+0*2+1] = 0x03 //
	p.lcdc = 0x90             // LCDC bit 0 clear: the background still shows
	p.ly = 0
	p.renderLine()
	if got := p.cback[7]; got != 0x001F {
		t.Errorf("flipped pixel 7 = %04X, want 001F", got)
	}
	if got := p.cback[0]; got != 0x0000 {
		t.Errorf("pixel 0 = %04X, want palette 3 color 0 (0000)", got)
	}
	if got := p.cback[8]; got != 0x03E0 {
		t.Errorf("pixel 8 = %04X, want palette 0 color 0 (03E0)", got)
	}
}

func TestCGBSaveState(t *testing.T) {
	g := newTestCGB(t)
	g.Bus.write(0xFF70, 5)
	g.Bus.write(0xD123, 0x42)
	g.Bus.write(0xFF4F, 1)
	g.Bus.write(0x8123, 0x24)
	g.Bus.write(0xFF68, 0x85)
	g.Bus.write(0xFF69, 0x11)
	g.Bus.doubleSpeed = true
	state := g.SaveState()

	g2 := newTestCGB(t)
	if err := g2.LoadState(state); err != nil {
		t.Fatal(err)
	}
	b := g2.Bus
	if b.read(0xD123) != 0x42 || b.read(0x8123) != 0x24 || !b.doubleSpeed ||
		g2.PPU.bgPal[5] != 0x11 || b.read(0xFF68) != 0xC6 {
		t.Error("CGB state not restored")
	}

	// A state of the other hardware mode is refused.
	dmg, _ := NewModel(g.Cart, nil, ModelDMG)
	if err := dmg.LoadState(state); err == nil {
		t.Error("CGB state loaded on a DMG")
	}
}

func TestLoadVersion1State(t *testing.T) {
	g := newTestGB(t)
	g.Bus.write(0xD000, 0x99)
	g.Bus.write(0x9FFF, 0x77)
	g.Bus.write(0xFF47, 0x1B)
	state := g.encodeState(1)

	g2 := newTestGB(t)
	if err := g2.LoadState(state); err != nil {
		t.Fatal(err)
	}
	if g2.Bus.read(0xD000) != 0x99 || g2.Bus.read(0x9FFF) != 0x77 || g2.PPU.bgp != 0x1B {
		t.Error("version 1 state not restored")
	}
}
