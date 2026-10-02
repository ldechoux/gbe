package gb

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

// compatROM builds a 32 KiB DMG-only ROM with the given title (up to 16
// bytes, 0x134-0x143) and old licensee code.
func compatROM(title string, licensee byte) []byte {
	rom := make([]byte, 0x8000)
	copy(rom[0x134:0x144], title)
	rom[0x14B] = licensee
	return rom
}

// newCompatGB runs a DMG-only game on a Game Boy Color, without boot ROM.
func newCompatGB(t *testing.T, rom []byte) *GameBoy {
	t.Helper()
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	g, err := NewModel(cart, nil, ModelCGB)
	if err != nil {
		t.Fatal(err)
	}
	if !g.IsCGB() || !g.Compat() {
		t.Fatal("DMG game not run in compatibility mode")
	}
	g.Bus.ifl = 0
	return g
}

// paletteRAM returns the colors a combination loads: OBJ0, OBJ1, BG.
func comboColors(combo int) []uint16 {
	var out []uint16
	for _, off := range compatCombos[combo] {
		out = append(out, compatColors[off:off+4]...)
	}
	return out
}

func loadedColors(g *GameBoy) []uint16 {
	var out []uint16
	for _, pal := range []struct {
		ram *[64]byte
		n   byte
	}{{&g.PPU.objPal, 0}, {&g.PPU.objPal, 1}, {&g.PPU.bgPal, 0}} {
		for i := range byte(4) {
			out = append(out, color(pal.ram, pal.n, i))
		}
	}
	return out
}

func TestCompatAutoCombo(t *testing.T) {
	for _, c := range []struct {
		title    string
		licensee byte
		combo    int
		checksum byte
	}{
		{"TETRIS", 0x01, 3, 0xDB},
		{"ZELDA", 0x01, 44, 0x70},
		{"POKEMON BLUE", 0x01, 11, 0x61}, // checksum shared, 4th letter E
		{"VEGAS STAKES", 0x01, 41, 0x61}, // same checksum, 4th letter A
		{"POKXMON BLU2", 0x01, 0, 0x61},  // same checksum, no such 4th letter
		{"TETRIS", 0x08, 0, 0},           // not from Nintendo
	} {
		cart, _ := NewCartridge(compatROM(c.title, c.licensee))
		if combo, sum := compatAutoCombo(cart); combo != c.combo || sum != c.checksum {
			t.Errorf("%s (licensee %02X): combo %d checksum %02X, want %d %02X",
				c.title, c.licensee, combo, sum, c.combo, c.checksum)
		}
	}

	// A Nintendo game missing from the table: default combination, but B
	// still gets the checksum.
	sum := byte(0)
	for bytes.IndexByte(compatChecksums[:], sum) >= 0 {
		sum++
	}
	title := titleFor(sum, 'Z')
	cart, _ := NewCartridge(compatROM(string(title[:]), 0x01))
	if combo, got := compatAutoCombo(cart); combo != 0 || got != sum {
		t.Errorf("unknown game: combo %d checksum %02X, want 0 %02X", combo, got, sum)
	}

	// The new licensee code "01" is Nintendo too.
	rom := compatROM("TETRIS", 0x33)
	rom[0x144], rom[0x145] = '0', '1'
	cart, _ = NewCartridge(rom)
	if combo, _ := compatAutoCombo(cart); combo != 3 {
		t.Errorf("new licensee 01: combo %d, want 3", combo)
	}
}

func TestCompatTables(t *testing.T) {
	if len(compatChecksums) != len(compatPalettePerChecksum) ||
		len(compatChecksums)-compatFirstDuplicate != len(compatLetters) {
		t.Fatalf("%d checksums, %d palettes, %d letters", len(compatChecksums),
			len(compatPalettePerChecksum), len(compatLetters))
	}
	for i, p := range compatPalettePerChecksum {
		if int(p) >= len(compatCombos) {
			t.Errorf("checksum %d: combination %d out of range", i, p)
		}
	}
	for i, c := range compatCombos {
		for _, off := range c {
			if int(off)+4 > len(compatColors) {
				t.Errorf("combination %d: offset %d out of range", i, off)
			}
		}
	}
}

func TestCompatSkipBoot(t *testing.T) {
	g := newCompatGB(t, compatROM("TETRIS", 0x01))
	c := g.CPU
	if c.a != 0x11 || c.f != 0x80 || c.b != 0xDB || c.c != 0 || c.d != 0 || c.e != 0x08 || c.h != 0 || c.l != 0x7C {
		t.Errorf("registers A=%02X F=%02X B=%02X C=%02X D=%02X E=%02X H=%02X L=%02X",
			c.a, c.f, c.b, c.c, c.d, c.e, c.h, c.l)
	}
	if got, want := loadedColors(g), comboColors(3); !slicesEqual(got, want) {
		t.Errorf("palettes %04X, want %04X", got, want)
	}
	if g.PPU.opri != 1 || g.Bus.cgbMode() {
		t.Error("compatibility mode not set up")
	}
	if c := color(&g.PPU.bgPal, 7, 0); c != 0x7FFF {
		t.Errorf("unused BG palette 7 = %04X, want white", c)
	}
}

func slicesEqual(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestKEY0(t *testing.T) {
	cart, _ := NewCartridge(compatROM("TETRIS", 0x01))
	g, err := NewModel(cart, make([]byte, cgbBootSize), ModelCGB)
	if err != nil {
		t.Fatal(err)
	}
	b := g.Bus
	if g.Compat() || !b.cgbMode() {
		t.Fatal("the boot ROM must run in CGB mode")
	}
	b.write(0xFF4C, 0x04)
	b.write(0xFF68, 0x80) // the boot ROM still loads palettes
	b.write(0xFF69, 0x12)
	if g.Compat() || g.PPU.bgPal[0] != 0x12 {
		t.Fatal("compatibility mode entered before the boot ROM ended")
	}
	b.write(0xFF50, 0x11)
	if !g.Compat() || b.cgbMode() {
		t.Fatal("KEY0=04 then FF50: not in compatibility mode")
	}
	for _, r := range []uint16{0xFF4C, 0xFF4D, 0xFF4F, 0xFF55, 0xFF68, 0xFF69, 0xFF6A, 0xFF6B, 0xFF6C, 0xFF70} {
		b.write(r, 0x01)
		if got := b.read(r); got != 0xFF {
			t.Errorf("register %04X reads %02X in compatibility mode, want FF", r, got)
		}
	}
	if g.PPU.bgPal[0] != 0x12 || g.PPU.vbk != 0 || b.svbk != 0 {
		t.Error("CGB registers written in compatibility mode")
	}
	b.write(0xFF4C, 0x00) // KEY0 is locked
	b.write(0xFF50, 0x01)
	if !g.Compat() {
		t.Error("compatibility mode left after the boot")
	}

	// A CGB game whose boot ROM leaves KEY0 alone stays in CGB mode.
	cgb := newTestCGB(t)
	cgb.Bus.boot, cgb.Bus.bootEnabled = make([]byte, cgbBootSize), true
	cgb.Bus.write(0xFF50, 0x11)
	if cgb.Compat() {
		t.Error("CGB game in compatibility mode")
	}
}

func TestCompatRendering(t *testing.T) {
	g := newCompatGB(t, compatROM("TETRIS", 0x01))
	p := g.PPU
	for i := range 16 {
		p.vram[0x10+i] = 0xFF // tile 1: color 3 everywhere
	}
	p.vram[0x1800] = 1                                         // first map tile
	p.vram[0x2000+0x1800] = 0x0F                               // CGB attributes (palette 7, bank 1): ignored
	p.oam[0], p.oam[1], p.oam[2], p.oam[3] = 16, 8+16, 1, 0x10 // tile 1 at x=16, OBP1
	p.lcdc, p.bgp, p.obp1 = 0x93, 0xE4, 0x1B
	p.ly = 0
	p.renderLine()

	line := p.cback[:ScreenWidth]
	if want := color(&p.bgPal, 0, 3); line[0] != want {
		t.Errorf("BG color 3: %04X, want BG palette 0 color 3 %04X", line[0], want)
	}
	if want := color(&p.bgPal, 0, 0); line[8] != want {
		t.Errorf("BG color 0: %04X, want %04X", line[8], want)
	}
	if want := color(&p.objPal, 1, shade(0x1B, 3)); line[16] != want {
		t.Errorf("sprite with OBP1: %04X, want OBJ palette 1 color 0 %04X", line[16], want)
	}
	if p.back[0] != 3 || p.back[16] != 0 {
		t.Errorf("DMG shades %d %d, want 3 0", p.back[0], p.back[16])
	}

	p.lcdc = 0x92 // LCDC.0 clear: the background is blank, as on a DMG
	p.renderLine()
	if want := color(&p.bgPal, 0, shade(0xE4, 0)); line[0] != want {
		t.Errorf("LCDC.0 off: %04X, want %04X", line[0], want)
	}
}

func TestSetCompatPalette(t *testing.T) {
	g := newCompatGB(t, compatROM("TETRIS", 0x01))
	g.SetCompatPalette(9) // Left + B
	if got, want := loadedColors(g), comboColors(int(compatKeyCombos[9])); !slicesEqual(got, want) {
		t.Errorf("Left+B: %04X, want %04X", got, want)
	}
	g.SetCompatPalette(CompatAuto)
	if got, want := loadedColors(g), comboColors(3); !slicesEqual(got, want) {
		t.Errorf("auto: %04X, want %04X", got, want)
	}

	cgb := newTestCGB(t)
	before := cgb.PPU.bgPal
	cgb.SetCompatPalette(0)
	if cgb.PPU.bgPal != before {
		t.Error("SetCompatPalette changed the palettes of a CGB game")
	}
}

func TestCompatSaveState(t *testing.T) {
	g := newCompatGB(t, compatROM("TETRIS", 0x01))
	g.SetCompatPalette(2)
	state := g.SaveState()
	if m, err := StateModel(state); err != nil || m != ModelCGB {
		t.Errorf("StateModel: %v %v", m, err)
	}

	g2 := newCompatGB(t, compatROM("TETRIS", 0x01))
	if err := g2.LoadState(state); err != nil {
		t.Fatal(err)
	}
	if !g2.Compat() || g2.PPU.bgPal != g.PPU.bgPal {
		t.Error("compatibility mode not restored")
	}

	dmg, _ := NewModel(g.Cart, nil, ModelDMG)
	if m, _ := StateModel(dmg.SaveState()); m != ModelDMG {
		t.Errorf("StateModel of a DMG state: %v", m)
	}
	if err := g.LoadState(dmg.SaveState()); err == nil {
		t.Error("DMG state loaded in compatibility mode")
	}
	if _, err := StateModel([]byte("garbage")); err == nil {
		t.Error("StateModel accepted garbage")
	}

	// Version 2 states predate the compatibility mode.
	cgb := newTestCGB(t)
	if err := cgb.LoadState(cgb.encodeState(2)); err != nil || cgb.Compat() {
		t.Errorf("version 2 state: %v, compat %v", err, cgb.Compat())
	}
}

// bootCompat runs the real CGB boot ROM on rom, holding the given buttons,
// until it hands over to the game.
func bootCompat(t *testing.T, boot, rom []byte, held ...Button) *GameBoy {
	t.Helper()
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	g, err := NewModel(cart, boot, ModelCGB)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range held {
		g.SetButton(b, true)
	}
	for i := 0; i < 600*CyclesPerFrame/4 && g.BootROMActive(); i++ {
		g.CPU.Step()
	}
	if g.BootROMActive() {
		t.Fatalf("boot ROM still active, PC=%04X", g.PC())
	}
	if !g.Compat() {
		t.Fatal("the boot ROM did not enter the compatibility mode")
	}
	return g
}

// withTitle copies base (a real ROM, for its logo) with another title and
// licensee, and fixes the header checksum the boot ROM verifies.
func withTitle(base []byte, title [16]byte, licensee byte) []byte {
	rom := bytes.Clone(base[:0x8000])
	copy(rom[0x134:0x144], title[:])
	rom[0x14B] = licensee
	var x byte
	for _, v := range rom[0x134:0x14D] {
		x = x - v - 1
	}
	rom[0x14D] = x
	return rom
}

// titleFor makes a title whose checksum is sum and 4th letter is letter.
func titleFor(sum, letter byte) (title [16]byte) {
	copy(title[:], "GBE")
	title[3] = letter
	rest := sum - 'G' - 'B' - 'E' - letter
	for i := 4; rest > 0; i++ { // bytes 4-15, ASCII and below 0x80 (DMG only)
		title[i] = min(rest, 0x5A)
		rest -= title[i]
	}
	return title
}

// TestCompatPalettesMatchBootROM checks the tables against the real boot
// ROM: every title of the table, and every button combination.
func TestCompatPalettesMatchBootROM(t *testing.T) {
	if testing.Short() {
		t.Skip("boots the real boot ROM about a hundred times")
	}
	boot, err := os.ReadFile("../../bios/gbc_bios.bin")
	if err != nil {
		t.Skip(err)
	}
	base, err := os.ReadFile("../../roms/Tetris_World_Rev1.gb")
	if err != nil {
		t.Skip(err) // any DMG ROM would do, for its logo
	}
	check := func(name string, rom []byte, held []Button, want func(*GameBoy)) {
		t.Run(name, func(t *testing.T) {
			real := bootCompat(t, boot, rom, held...)
			emu := newCompatGB(t, rom)
			want(emu)
			if got, want := loadedColors(real), loadedColors(emu); !slicesEqual(got, want) {
				t.Errorf("boot ROM palettes %04X, emulated %04X", got, want)
			}
			if len(held) == 0 && real.CPU.b != emu.CPU.b {
				t.Errorf("B=%02X after the boot ROM, %02X emulated", real.CPU.b, emu.CPU.b)
			}
		})
	}
	for i, sum := range compatChecksums {
		letter := byte('Z')
		if i >= compatFirstDuplicate {
			letter = compatLetters[i-compatFirstDuplicate]
		}
		rom := withTitle(base, titleFor(sum, letter), 0x01)
		check(fmt.Sprintf("checksum %d (%02X)", i, sum), rom, nil, func(*GameBoy) {})
	}
	check("not Nintendo", withTitle(base, titleFor(0xDB, 'R'), 0x08), nil, func(*GameBoy) {})

	dirs := []Button{ButtonRight, ButtonLeft, ButtonUp, ButtonDown}
	for i := range CompatKeyPalettes {
		held := []Button{dirs[i%4]}
		switch i / 4 {
		case 1:
			held = append(held, ButtonA)
		case 2:
			held = append(held, ButtonB)
		}
		check(fmt.Sprintf("buttons %d", i), base, held, func(g *GameBoy) { g.SetCompatPalette(i) })
	}
}
