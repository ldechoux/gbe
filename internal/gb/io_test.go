package gb

import "testing"

func TestTimerRegisters(t *testing.T) {
	g := newTestGB(t)
	tm := g.Timer

	tm.counter = 0x1234
	if got := g.Bus.read(0xFF04); got != 0x12 {
		t.Errorf("DIV %02X, want 12", got)
	}
	g.Bus.write(0xFF06, 0x42)
	g.Bus.write(0xFF07, 0xFD) // only the low 3 bits are stored
	if tma, tac := g.Bus.read(0xFF06), g.Bus.read(0xFF07); tma != 0x42 || tac != 0xFD {
		t.Errorf("TMA=%02X TAC=%02X, want 42 FD", tma, tac)
	}

	// Writing DIV resets the counter. If the bit selected by TAC was set,
	// that is a falling edge and TIMA increments.
	g.Bus.write(0xFF07, 0x05) // enabled, bit 3
	g.Bus.write(0xFF05, 0x10)
	tm.counter = 0x0008
	g.Bus.write(0xFF04, 0xAB)
	if tm.counter != 0 || g.Bus.read(0xFF05) != 0x11 {
		t.Errorf("DIV reset with bit 3 set: counter %#x TIMA %02X, want 0 11", tm.counter, tm.tima)
	}
	g.Bus.write(0xFF04, 0x00)
	if got := g.Bus.read(0xFF05); got != 0x11 {
		t.Errorf("DIV reset with bit 3 clear changed TIMA to %02X", got)
	}

	// Disabling the timer while the selected bit is set is a falling edge too.
	tm.counter = 0x0008
	g.Bus.write(0xFF07, 0x01)
	if got := g.Bus.read(0xFF05); got != 0x12 {
		t.Errorf("TAC disable: TIMA %02X, want 12", got)
	}

	// Writing TIMA during the overflow cycle cancels the reload.
	tm.tima = 0xFF
	tm.incTIMA()
	g.Bus.write(0xFF05, 0x33)
	tm.tick()
	if tm.tima != 0x33 || g.Bus.ifl&IntTimer != 0 {
		t.Errorf("TIMA %02X IF %02X, want 33 without a timer interrupt", tm.tima, g.Bus.ifl)
	}
}

func TestSerial(t *testing.T) {
	g := newTestGB(t)
	g.Bus.write(0xFF01, 'X')
	if got := g.Bus.read(0xFF01); got != 'X' {
		t.Errorf("SB %02X, want %02X", got, 'X')
	}
	g.Bus.write(0xFF02, 0x80) // external clock: nothing happens
	if g.Bus.read(0xFF02) != 0xFE || len(g.Serial.Output) != 0 {
		t.Errorf("external clock: SC %02X output %q", g.Bus.read(0xFF02), g.Serial.Output)
	}

	g.Bus.write(0xFF02, 0x81)
	if string(g.Serial.Output) != "X" {
		t.Errorf("output %q, want X", g.Serial.Output)
	}
	for range 8*512/4 - 1 {
		g.Serial.tick()
	}
	if g.Bus.ifl&IntSerial != 0 || g.Bus.read(0xFF02) != 0xFF {
		t.Fatal("transfer completed early")
	}
	g.Serial.tick()
	if g.Bus.ifl&IntSerial == 0 {
		t.Error("serial interrupt not requested")
	}
	if sb, sc := g.Bus.read(0xFF01), g.Bus.read(0xFF02); sb != 0xFF || sc != 0x7F {
		t.Errorf("after transfer SB=%02X SC=%02X, want FF 7F", sb, sc)
	}
}

func TestPPURegisters(t *testing.T) {
	g := newTestGB(t)
	g.Bus.write(0xFF40, 0x11) // LCD off, so nothing moves
	for _, addr := range []uint16{0xFF42, 0xFF43, 0xFF45, 0xFF47, 0xFF48, 0xFF49, 0xFF4A, 0xFF4B} {
		v := byte(addr) ^ 0x5A
		g.Bus.write(addr, v)
		if got := g.Bus.read(addr); got != v {
			t.Errorf("%04X reads %02X, want %02X", addr, got, v)
		}
	}
	if got := g.Bus.read(0xFF40); got != 0x11 {
		t.Errorf("LCDC %02X, want 11", got)
	}
	g.Bus.write(0xFF44, 0x99) // LY is read-only
	if got := g.Bus.read(0xFF44); got != 0 {
		t.Errorf("LY %02X, want 0", got)
	}
	g.Bus.write(0xFF46, 0xC0) // DMA source
	if got := g.Bus.read(0xFF46); got != 0xC0 {
		t.Errorf("DMA %02X, want C0", got)
	}

	// STAT: bit 7 always set, the mode and coincidence bits are read-only.
	g.Bus.write(0xFF45, 0x00)
	g.Bus.write(0xFF41, 0xFF)
	if got := g.Bus.read(0xFF41); got != 0xFC {
		t.Errorf("STAT %02X, want FC (LY=LYC, mode 0)", got)
	}
	g.Bus.write(0xFF45, 0x01)
	g.PPU.mode = 3
	if got := g.Bus.read(0xFF41); got != 0xFB {
		t.Errorf("STAT %02X, want FB (LY!=LYC, mode 3)", got)
	}
	if got := g.Bus.read(0xFF4C); got != 0xFF {
		t.Errorf("unmapped 0xFF4C reads %02X, want FF", got)
	}
}

func TestBusMemoryMap(t *testing.T) {
	g := newTestGB(t)
	b := g.Bus
	for _, c := range []struct {
		name        string
		write, read uint16
	}{
		{"VRAM", 0x8123, 0x8123},
		{"WRAM", 0xC123, 0xC123},
		{"echo RAM", 0xE123, 0xC123},
		{"OAM", 0xFE12, 0xFE12},
		{"HRAM", 0xFF90, 0xFF90},
		{"IE", 0xFFFF, 0xFFFF},
	} {
		b.write(c.write, 0xA7)
		if got := b.read(c.read); got != 0xA7 {
			t.Errorf("%s: %04X reads %02X, want A7", c.name, c.read, got)
		}
	}
	b.write(0xFEA0, 0x12)
	if got := b.read(0xFEA0); got != 0xFF {
		t.Errorf("unusable area reads %02X, want FF", got)
	}
	b.write(0xFF0F, 0xFF)
	if got := b.read(0xFF0F); got != 0xFF || b.ifl != 0x1F {
		t.Errorf("IF reads %02X (stored %02X), want FF (1F)", got, b.ifl)
	}
	if got := b.read(0xFF03); got != 0xFF {
		t.Errorf("unmapped 0xFF03 reads %02X, want FF", got)
	}
}

func TestBootROMOverlay(t *testing.T) {
	rom := make([]byte, 0x8000)
	rom[0x00], rom[0x100] = 0xC3, 0x00
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(cart, make([]byte, 0x80)); err == nil {
		t.Error("boot ROM of the wrong size accepted")
	}
	boot := make([]byte, 0x100)
	boot[0x00] = 0x31
	g, err := New(cart, boot)
	if err != nil {
		t.Fatal(err)
	}
	if g.Bus.read(0x0000) != 0x31 || g.Bus.read(0x0100) != 0x00 {
		t.Error("boot ROM not mapped over 0x0000-0x00FF")
	}
	g.Bus.write(0xFF50, 0x00) // only a non-zero write unmaps it
	if g.Bus.read(0x0000) != 0x31 {
		t.Error("writing 0 to 0xFF50 unmapped the boot ROM")
	}
	g.Bus.write(0xFF50, 0x01)
	if g.Bus.read(0x0000) != 0xC3 {
		t.Error("boot ROM still mapped after writing 0xFF50")
	}
}

func TestJoypad(t *testing.T) {
	g := newTestGB(t)
	g.Bus.ifl = 0
	g.SetButton(ButtonA, true)
	g.SetButton(ButtonLeft, true)

	g.Bus.write(0xFF00, 0x10) // select action keys
	if got := g.Bus.read(0xFF00); got != 0xDE {
		t.Errorf("action keys %02X, want DE (A pressed)", got)
	}
	g.Bus.write(0xFF00, 0x20) // select direction keys
	if got := g.Bus.read(0xFF00); got != 0xED {
		t.Errorf("direction keys %02X, want ED (Left pressed)", got)
	}
	g.Bus.write(0xFF00, 0x30)
	if got := g.Bus.read(0xFF00); got != 0xFF {
		t.Errorf("nothing selected %02X, want FF", got)
	}

	g.Bus.ifl = 0
	g.SetButton(ButtonDown, true) // direction keys not selected: no interrupt
	if g.Bus.ifl&IntJoypad != 0 {
		t.Error("joypad interrupt for an unselected key")
	}
	g.Bus.write(0xFF00, 0x20)
	g.SetButton(ButtonUp, true)
	if g.Bus.ifl&IntJoypad == 0 {
		t.Error("joypad interrupt not requested")
	}

	names := map[string]bool{}
	for _, b := range Buttons {
		names[b.String()] = true
	}
	if len(names) != 8 || names[""] {
		t.Errorf("button names %v, want 8 distinct names", names)
	}
}
