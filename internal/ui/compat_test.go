package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ldechoux/gbe/internal/gb"
)

// withCompatGameBoy gives g a DMG game colorized on a Game Boy Color. Its
// screen shows color 2 of the BG palette, which differs between palettes.
func withCompatGameBoy(t *testing.T, g *Game) *Game {
	t.Helper()
	rom := make([]byte, 0x8000)
	copy(rom[0x100:], []byte{0x3E, 0x02, 0xE0, 0x47, 0x18, 0xFE}) // LD A,2; LDH (BGP),A; JR $
	copy(rom[0x134:], "TETRIS")
	rom[0x14B] = 0x01 // Nintendo: the palette depends on the title
	cart, err := gb.NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	g.gb, err = gb.NewModel(cart, nil, gb.ModelCGB)
	if err != nil || !g.gb.Compat() {
		t.Fatalf("not in compatibility mode: %v", err)
	}
	g.title = cart.Title
	g.stream = &audioStream{}
	dir := t.TempDir()
	g.statePath, g.savePath = filepath.Join(dir, "game.state"), filepath.Join(dir, "game.sav")
	return g
}

func TestCompatPaletteIDs(t *testing.T) {
	l := newTestGame(t, newFakePads()).tr()
	for i, want := range []struct{ id, label string }{
		{"right", "Right"}, {"left", "Left"}, {"up", "Up"}, {"down", "Down"},
		{"right+a", "Right+A"}, {"left+a", "Left+A"}, {"up+a", "Up+A"}, {"down+a", "Down+A"},
		{"right+b", "Right+B"}, {"left+b", "Left+B"}, {"up+b", "Up+B"}, {"down+b", "Down+B"},
	} {
		if id, label := compatPaletteID(i), compatPaletteLabel(l, i); id != want.id || label != want.label {
			t.Errorf("palette %d: %q %q, want %q %q", i, id, label, want.id, want.label)
		}
		if compatPaletteIndex(want.id) != i {
			t.Errorf("index of %q: %d, want %d", want.id, compatPaletteIndex(want.id), i)
		}
	}
	if compatPaletteIndex("nope") != gb.CompatAuto || compatPaletteLabel(l, gb.CompatAuto) != "Auto" {
		t.Error("unknown ids must mean Auto")
	}
}

func TestCompatCyclePalette(t *testing.T) {
	g := withCompatGameBoy(t, newTestGame(t, newFakePads()))
	g.gb.RunFrame()
	g.gb.RunFrame()
	auto := *g.gb.ColorFramebuffer()
	correction, palette := g.cfg.ColorCorrection, g.cfg.Palette

	g.cyclePalette(-1) // from Auto, back to the last combination
	if got := g.cfg.CompatPalettes["TETRIS"]; got != "down+b" {
		t.Errorf("Left from Auto: %q, want down+b", got)
	}
	if g.cfg.ColorCorrection != correction || g.cfg.Palette != palette {
		t.Error("the color correction or the DMG palette changed")
	}
	g.gb.RunFrame()
	if *g.gb.ColorFramebuffer() == auto {
		t.Error("the new palette is not applied at once")
	}
	if cfg, _ := LoadConfig(g.cfgPath); cfg.CompatPalettes["TETRIS"] != "down+b" {
		t.Errorf("choice not saved: %v", cfg.CompatPalettes)
	}
	if got := paletteLabel(g.tr(), g); got != "Palette   < Down+B >" {
		t.Errorf("menu entry %q", got)
	}

	g.cyclePalette(1)
	if _, ok := g.cfg.CompatPalettes["TETRIS"]; ok || paletteLabel(g.tr(), g) != "Palette   < Auto >" {
		t.Errorf("back to Auto: %v", g.cfg.CompatPalettes)
	}
	g.gb.RunFrame()
	if *g.gb.ColorFramebuffer() != auto {
		t.Error("Auto does not bring the title's palette back")
	}
	g.cyclePalette(1)
	if got := g.cfg.CompatPalettes["TETRIS"]; got != "right" {
		t.Errorf("Right from Auto: %q, want right", got)
	}
}

func TestCompatChoiceReapplied(t *testing.T) {
	g := withCompatGameBoy(t, newTestGame(t, newFakePads()))
	g.gb.RunFrame()
	auto := *g.gb.ColorFramebuffer()
	g.applyCompatChoice()
	g.gb.RunFrame()
	if *g.gb.ColorFramebuffer() != auto {
		t.Fatal("Auto must leave the palette alone")
	}

	g.cfg.CompatPalettes["TETRIS"] = "left+b"
	g.applyCompatChoice()
	g.gb.RunFrame()
	chosen := *g.gb.ColorFramebuffer()
	if chosen == auto {
		t.Fatal("the test needs palettes that differ")
	}
	g.gb.SetCompatPalette(gb.CompatAuto) // e.g. an older state, or the boot ROM
	g.applyCompatChoice()
	g.gb.RunFrame()
	if *g.gb.ColorFramebuffer() != chosen {
		t.Error("the chosen palette is not applied again")
	}
}

func TestCompatMenu(t *testing.T) {
	g := withCompatGameBoy(t, newTestGame(t, newFakePads()))
	l := g.tr()
	if _, _, footer := g.menu.lines(g); footer != l.T("menu.footer") {
		t.Errorf("footer %q: P picks a palette", footer)
	}
	if got := g.menuPalette().ID; got != "grey" {
		t.Errorf("menu palette %q, want grey", got)
	}

	if !g.cfg.ColorizeDMG {
		t.Fatal("colorization off by default")
	}
	g.menu.show()
	g.menu.cursor = itemColorize
	g.actions = menuActions{ok: true}
	g.menu.update(g)
	if g.cfg.ColorizeDMG || g.toast != "Applied at next launch" {
		t.Errorf("colorize entry: %v, toast %q", g.cfg.ColorizeDMG, g.toast)
	}
	if cfg, _ := LoadConfig(g.cfgPath); cfg.ColorizeDMG {
		t.Error("setting not saved")
	}
	if _, items, _ := g.menu.lines(g); items[itemColorize] != "Colorize  < off >" {
		t.Errorf("entry %q", items[itemColorize])
	}
}

func TestColorizeConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"colorize_dmg": false, "compat_palettes": {"TETRIS": "up+a", "ZELDA": "nope"}}`), 0o644)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ColorizeDMG || cfg.CompatPalettes["TETRIS"] != "up+a" || len(cfg.CompatPalettes) != 1 {
		t.Errorf("loaded colorize %v palettes %v", cfg.ColorizeDMG, cfg.CompatPalettes)
	}
	os.WriteFile(path, []byte(`{}`), 0o644)
	if cfg, _ := LoadConfig(path); !cfg.ColorizeDMG || cfg.CompatPalettes == nil {
		t.Errorf("defaults: colorize %v palettes %v", cfg.ColorizeDMG, cfg.CompatPalettes)
	}
}

func TestStartOverInOtherMode(t *testing.T) {
	g := withGameBoy(t, newTestGame(t, newFakePads()))
	g.saveState()
	dmg := g.gb
	built := 0
	g.freshStart = func() (*gb.GameBoy, error) {
		built++
		return withCompatGameBoy(t, &Game{}).gb, nil
	}
	m := &g.menu

	m.showStart()
	g.actions = menuActions{ok: true} // Resume, in the mode of the state
	m.update(g)
	if g.gb != dmg || built != 0 || g.toast != "Saved in the other mode: restart to switch" {
		t.Errorf("resume: console replaced %v, toast %q", g.gb != dmg, g.toast)
	}

	m.showStart()
	m.cursor = startFresh
	m.update(g)
	if built != 1 || !g.gb.Compat() || !g.started {
		t.Errorf("start over: built %d, compat %v", built, g.gb.Compat())
	}
}
