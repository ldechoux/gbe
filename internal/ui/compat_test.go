package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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
}

// withModes lets the menu rebuild the console in another mode, as when the
// hardware is left to auto.
func withModes(g *Game) *Game {
	g.autoModel = true
	g.newConsole = func(m gb.Model) (*gb.GameBoy, error) { return gb.NewModel(g.gb.Cart, nil, m) }
	return g
}

// entryLabel is the label of a main page entry.
func entryLabel(g *Game, item int) string {
	m := menu{page: pageMain}
	_, items, _ := m.lines(g)
	return items[item]
}

// displayLabel is the label of a display page entry, "" when it is hidden.
func displayLabel(g *Game, item int) string {
	m := menu{page: pageDisplay}
	_, items, _ := m.lines(g)
	if i := slices.Index(displayEntries(g), item); i >= 0 {
		return items[i]
	}
	return ""
}

func TestColorizeEntryShown(t *testing.T) {
	g := withModes(withCompatGameBoy(t, newTestGame(t, newFakePads())))
	g.cfg.ColorizeDMG = true
	if displayLabel(g, displayColorize) != "Colorize  < on >" {
		t.Errorf("DMG game left to auto: entry %q", displayLabel(g, displayColorize))
	}

	g.autoModel = false // -model dmg or cgb decides
	if displayLabel(g, displayColorize) != "" {
		t.Error("entry shown although -model forces the hardware")
	}
	g.menu = menu{open: true, page: pageDisplay, cursor: displayPalette}
	g.actions = menuActions{down: true}
	g.menu.update(g)
	if g.menu.cursor != displayScale || g.menu.view(g).selected != 1 {
		t.Errorf("Down skips the hidden entry: cursor %d, selected %d", g.menu.cursor, g.menu.view(g).selected)
	}
	g.actions = menuActions{up: true}
	g.menu.update(g)
	if g.menu.cursor != displayPalette {
		t.Errorf("Up skips the hidden entry: cursor %d", g.menu.cursor)
	}

	c := withModes(withColorGameBoy(t, newTestGame(t, newFakePads())))
	if displayLabel(c, displayColorize) != "" || len(displayEntries(c)) != displayItems-1 {
		t.Error("entry shown for a Game Boy Color game")
	}
}

func TestResetAppliesColorize(t *testing.T) {
	g := withModes(withCompatGameBoy(t, newTestGame(t, newFakePads())))
	g.cfg.ColorizeDMG = true
	l := g.tr()
	m := &g.menu
	press := func(item int) {
		m.show()
		m.cursor = item
		g.actions = menuActions{ok: true}
		m.update(g)
	}
	colorize := func() {
		*m = menu{open: true, page: pageDisplay, cursor: displayColorize}
		g.actions = menuActions{ok: true}
		m.update(g)
	}

	colorize()
	if g.cfg.ColorizeDMG || displayLabel(g, displayColorize) != "Colorize  < off >" {
		t.Fatalf("entry: %v %q", g.cfg.ColorizeDMG, displayLabel(g, displayColorize))
	}
	if cfg, _ := LoadConfig(g.cfgPath); cfg.ColorizeDMG {
		t.Error("setting not saved")
	}
	if !g.gb.Compat() || !g.modePending() {
		t.Fatal("the game must stay colorized until reset")
	}
	if _, _, footer := m.lines(g); footer != l.T("menu.footer_pending") {
		t.Errorf("footer %q, want the reset hint", footer)
	}
	m.backToMain()
	if _, _, footer := m.lines(g); footer != l.T("menu.footer_pending") {
		t.Errorf("main page footer %q, want the reset hint", footer)
	}
	colorize()
	if g.modePending() {
		t.Error("back to the running mode: nothing pending")
	}

	colorize()
	press(itemReset)
	if g.gb.IsCGB() || g.modePending() {
		t.Fatal("reset must apply the setting: DMG")
	}
	if _, _, footer := m.lines(g); footer != l.T("menu.footer") {
		t.Errorf("footer after reset %q", footer)
	}
	colorize()
	press(itemReset)
	if !g.gb.Compat() {
		t.Fatal("reset must apply the setting: colorized")
	}
	g.gb.RunFrame()
	press(itemReset) // nothing pending: the same console, power-cycled
	if !g.gb.Compat() || g.gb.PC() != 0x0100 {
		t.Errorf("plain reset: compat %v PC %04X", g.gb.Compat(), g.gb.PC())
	}
}

func TestStartPageOtherMode(t *testing.T) {
	g := withModes(withGameBoy(t, newTestGame(t, newFakePads()), busyProgram...))
	g.cfg.ColorizeDMG = true
	g.saveState()
	dmg := g.gb
	m := &g.menu

	// A DMG state, while the settings ask for colors.
	m.showStart()
	_, _, footer := m.lines(g)
	if lines := strings.Split(footer, "\n"); len(lines) != 2 || lines[1] != "DMG game: start over to colorize" {
		t.Errorf("start page footer %q", footer)
	}
	g.actions = menuActions{ok: true} // Resume, in the mode of the state
	m.update(g)
	if g.gb != dmg || g.toast != "Game restored" {
		t.Errorf("resume: console replaced %v, toast %q", g.gb != dmg, g.toast)
	}

	m.showStart()
	m.cursor = startFresh
	m.update(g)
	if !g.gb.Compat() || !g.started {
		t.Errorf("start over: compat %v", g.gb.Compat())
	}

	m.showStart()
	if _, _, footer := m.lines(g); strings.Contains(footer, "\n") {
		t.Errorf("same mode: footer %q", footer)
	}
	g.cfg.ColorizeDMG = false
	if _, _, footer := m.lines(g); !strings.HasSuffix(footer, "\nColorized game: start over for the DMG") {
		t.Errorf("colorized state, colorization off: footer %q", footer)
	}
}

func TestColorizeConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"colorize_dmg": true, "compat_palettes": {"TETRIS": "up+a", "ZELDA": "nope"}}`), 0o644)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ColorizeDMG || cfg.CompatPalettes["TETRIS"] != "up+a" || len(cfg.CompatPalettes) != 1 {
		t.Errorf("loaded colorize %v palettes %v", cfg.ColorizeDMG, cfg.CompatPalettes)
	}
	os.WriteFile(path, []byte(`{}`), 0o644)
	if cfg, _ := LoadConfig(path); cfg.ColorizeDMG || cfg.CompatPalettes == nil {
		t.Errorf("defaults: colorize %v palettes %v", cfg.ColorizeDMG, cfg.CompatPalettes)
	}
}
