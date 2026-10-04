package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/ui/scaler"
)

// withGameBoy gives g a machine running program from 0x0100 on an MBC1
// cartridge with battery-backed RAM, and a save state path.
func withGameBoy(t *testing.T, g *Game, program ...byte) *Game {
	t.Helper()
	rom := make([]byte, 0x8000)
	copy(rom[0x134:], "MENU TEST")
	rom[0x147], rom[0x149] = 0x03, 0x02
	copy(rom[0x100:], program)
	cart, err := gb.NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	g.gb, err = gb.New(cart, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	g.statePath = filepath.Join(dir, "game.state")
	g.savePath = filepath.Join(dir, "game.sav")
	return g
}

func TestMenuNavigation(t *testing.T) {
	g := newTestGame(t, newFakePads())
	g.menu.show()
	m := &g.menu

	g.actions = menuActions{up: true}
	m.update(g)
	if m.cursor != mainItems-1 {
		t.Errorf("Up from the first entry: cursor %d, want %d", m.cursor, mainItems-1)
	}
	g.actions = menuActions{down: true}
	m.update(g)
	if m.cursor != 0 {
		t.Errorf("Down from the last entry: cursor %d, want 0", m.cursor)
	}

	for _, c := range []struct {
		name string
		page menuPage
		tab  controlsTab
		want int
	}{
		{"main", pageMain, tabKeyboard, mainItems},
		{"display", pageDisplay, tabKeyboard, displayItems},
		{"start", pageStart, tabKeyboard, startItems},
		{"keyboard", pageControls, tabKeyboard, len(bindingNames()) + 3},
		{"gamepad", pageControls, tabPad, len(bindingNames()) + 4}, // vibration, test, defaults, back
	} {
		m := menu{page: c.page, tab: c.tab}
		if got := m.itemCount(); got != c.want {
			t.Errorf("%s page: %d entries, want %d", c.name, got, c.want)
		}
	}
}

func TestMenuBackAndToggle(t *testing.T) {
	g := newTestGame(t, newFakePads())
	m := &g.menu

	m.show()
	g.actions = menuActions{back: true}
	m.update(g)
	if m.open {
		t.Error("Back on the main page must close the menu")
	}

	*m = menu{open: true, page: pageControls, cursor: 3, notice: "x", width: 99}
	m.update(g)
	if !m.open || m.page != pageMain || m.cursor != itemControls || m.notice != "" || m.width != 0 {
		t.Errorf("Back on the controls page: %+v, want the main page on Controls", *m)
	}

	*m = menu{open: true, page: pageDisplay, cursor: displayFilter, width: 99}
	m.update(g)
	if !m.open || m.page != pageMain || m.cursor != itemDisplay || m.width != 0 {
		t.Errorf("Back on the display page: %+v, want the main page on Display", *m)
	}

	m.showStart()
	m.update(g)
	g.actions = menuActions{toggle: true}
	m.update(g)
	if !m.open {
		t.Error("the start page must stay open until a choice is made")
	}

	m.show()
	m.update(g)
	if m.open {
		t.Error("Start+Select must close the menu")
	}
}

func TestMenuActivateMain(t *testing.T) {
	g := withGameBoy(t, newTestGame(t, newFakePads()))
	m := &g.menu
	ok := func(cursor int) {
		m.cursor = cursor
		g.actions = menuActions{ok: true}
		m.update(g)
	}

	m.show()
	ok(itemResume)
	if m.open {
		t.Error("Resume must close the menu")
	}

	m.show()
	m.width = 50
	ok(itemControls)
	if m.page != pageControls || m.cursor != 0 || m.width != 0 {
		t.Errorf("Controls: %+v", *m)
	}

	m.show()
	m.width = 50
	ok(itemDisplay)
	if m.page != pageDisplay || m.cursor != displayPalette || m.width != 0 {
		t.Errorf("Display: %+v", *m)
	}
	if title, items, _ := m.lines(g); title != "DISPLAY" || items[len(items)-1] != "Back" {
		t.Errorf("display page: %q %q", title, items)
	}
	ok(displayPalette) // OK cycles like Right
	if g.cfg.Palette != Palettes[1].ID {
		t.Errorf("palette %q, want %q", g.cfg.Palette, Palettes[1].ID)
	}
	ok(displayBack)
	if m.page != pageMain || m.cursor != itemDisplay || !m.open {
		t.Errorf("Back entry: %+v, want the main page on Display", *m)
	}

	m.show()
	ok(itemLoadState)
	if g.toast != "No saved state" || m.open {
		t.Errorf("load without a state: toast %q open %v", g.toast, m.open)
	}

	m.show()
	ok(itemSaveState)
	if g.toast != "State saved" || m.open || g.stateTime.IsZero() {
		t.Errorf("save: toast %q open %v", g.toast, m.open)
	}
	if _, err := os.Stat(g.statePath + ".tmp"); err == nil {
		t.Error("temporary state file left behind")
	}
	pc := g.gb.PC()
	g.gb.RunFrame()
	m.show()
	ok(itemLoadState)
	if g.toast != "Game restored" || g.gb.PC() != pc {
		t.Errorf("load: toast %q PC %04X, want %04X", g.toast, g.gb.PC(), pc)
	}

	os.WriteFile(g.statePath, []byte("garbage"), 0o644)
	m.show()
	ok(itemLoadState)
	if !strings.HasPrefix(g.toast, "Unreadable state: ") {
		t.Errorf("load of a corrupt state: toast %q", g.toast)
	}

	g.gb.RunFrame()
	m.show()
	ok(itemReset)
	if g.gb.PC() != 0x0100 || m.open {
		t.Errorf("reset: PC %04X open %v", g.gb.PC(), m.open)
	}

	m.show()
	ok(itemQuit)
	if !g.quit || m.open {
		t.Errorf("quit: quit %v open %v", g.quit, m.open)
	}
}

func TestSaveStateFailure(t *testing.T) {
	g := withGameBoy(t, newTestGame(t, newFakePads()))
	g.statePath = filepath.Join(g.statePath, "missing-dir", "game.state")
	g.saveState()
	if !strings.HasPrefix(g.toast, "Cannot save: ") || !g.stateTime.IsZero() {
		t.Errorf("toast %q stateTime %v", g.toast, g.stateTime)
	}

	g.statePath, g.toast = "", ""
	g.saveState()
	if g.toast != "" {
		t.Errorf("without a state path: toast %q", g.toast)
	}
}

func TestMenuStartPage(t *testing.T) {
	g := withGameBoy(t, newTestGame(t, newFakePads()))
	g.saveState()
	g.gb.RunFrame()
	pc := g.gb.PC()
	m := &g.menu

	m.showStart()
	m.cursor = startFresh
	g.actions = menuActions{ok: true}
	m.update(g)
	if m.open || !g.started || g.gb.PC() != pc {
		t.Errorf("Start over: open %v started %v PC %04X", m.open, g.started, g.gb.PC())
	}

	g.started = false
	m.showStart()
	m.update(g) // cursor on Resume
	if m.open || !g.started || g.toast != "Game restored" {
		t.Errorf("Resume: open %v started %v toast %q", m.open, g.started, g.toast)
	}

	g.stateTime = time.Date(2026, 3, 4, 5, 6, 0, 0, time.Local)
	m.showStart()
	if title, items, footer := m.lines(g); title != "GAME IN PROGRESS" || len(items) != startItems ||
		footer != "Saved on 03/04/2026 at 05:06" {
		t.Errorf("start page: %q %q %q", title, items, footer)
	}
	g.stateTime = time.Time{}
	if _, _, footer := m.lines(g); footer != "Enter: confirm" {
		t.Errorf("footer without a date: %q", footer)
	}
}

func TestMenuAdjust(t *testing.T) {
	g := newTestGame(t, newFakePads())
	m := &g.menu
	m.show()
	adjust := func(cursor, delta int) {
		m.cursor = cursor
		if delta < 0 {
			g.actions = menuActions{left: true}
		} else {
			g.actions = menuActions{right: true}
		}
		m.update(g)
	}
	m.page = pageDisplay
	adjustDisplay := func(cursor, delta int) {
		m.cursor = cursor
		if delta < 0 {
			g.actions = menuActions{left: true}
		} else {
			g.actions = menuActions{right: true}
		}
		m.update(g)
	}

	adjustDisplay(displayPalette, -1)
	if g.cfg.Palette != Palettes[len(Palettes)-1].ID {
		t.Errorf("Left on the first palette: %q, want the last one", g.cfg.Palette)
	}
	adjustDisplay(displayPalette, 1)
	if g.cfg.Palette != Palettes[0].ID {
		t.Errorf("Right on the last palette: %q, want the first one", g.cfg.Palette)
	}

	g.cfg.Scale = maxScale
	adjustDisplay(displayScale, 1)
	if g.cfg.Scale != maxScale {
		t.Errorf("scale %d, want clamped to %d", g.cfg.Scale, maxScale)
	}
	g.cfg.Scale = 1
	adjustDisplay(displayScale, -1)
	if g.cfg.Scale != 1 {
		t.Errorf("scale %d, want clamped to 1", g.cfg.Scale)
	}
	adjustDisplay(displayScale, 1)
	if cfg, _ := LoadConfig(g.cfgPath); cfg.Scale != 2 {
		t.Errorf("saved scale %d, want 2", cfg.Scale)
	}

	adjustDisplay(displayFilter, -1)
	if last := scaler.Filters[len(scaler.Filters)-1].ID; g.cfg.Filter != last {
		t.Errorf("Left on the first filter: %q, want the last one", g.cfg.Filter)
	}
	adjustDisplay(displayFilter, 1)
	adjustDisplay(displayFilter, 1)
	if cfg, _ := LoadConfig(g.cfgPath); cfg.Filter != scaler.Filters[1].ID {
		t.Errorf("saved filter %q, want %q", cfg.Filter, scaler.Filters[1].ID)
	}
	if got := displayLabel(g, displayFilter); got != "Filter    < Sharp >" {
		t.Errorf("filter entry %q", got)
	}
	for _, want := range []string{"simple", "accurate", "off"} {
		adjustDisplay(displayGhosting, 1)
		if cfg, _ := LoadConfig(g.cfgPath); cfg.Ghosting != want {
			t.Errorf("Right on Ghosting: saved %q, want %q", cfg.Ghosting, want)
		}
	}
	adjustDisplay(displayGhosting, -1) // from off, wraps around
	if g.cfg.Ghosting != ghostingAccurate || displayLabel(g, displayGhosting) != "Ghosting  < accurate >" {
		t.Errorf("Left on Ghosting off: %q, entry %q", g.cfg.Ghosting, displayLabel(g, displayGhosting))
	}
	g.cfg.Ghosting = ghostingOff
	if got := displayLabel(g, displayGhosting); got != "Ghosting  < off >" {
		t.Errorf("ghosting off entry %q", got)
	}
	before := *g.cfg
	adjustDisplay(displayBack, 1) // not adjustable
	if g.cfg.Palette != before.Palette || g.cfg.Scale != before.Scale || g.cfg.Filter != before.Filter || m.page != pageDisplay {
		t.Error("Right on Back did something")
	}

	m.show()
	adjust(itemResume, 1) // not adjustable
	if g.cfg.Palette != before.Palette || g.cfg.Scale != before.Scale {
		t.Error("Right on Resume changed the settings")
	}
	m.showStart()
	adjust(startFresh, 1)
	if m.cursor != startFresh || !m.open {
		t.Error("Right on the start page did something")
	}
}

func TestMenuControlsActivate(t *testing.T) {
	pads := newFakePads("Xbox Wireless Controller")
	g := newTestGame(t, pads)
	m := &g.menu
	ok := func() {
		g.actions = menuActions{ok: true}
		m.update(g)
	}

	*m = menu{open: true, page: pageControls, cursor: controlsScreenshot()}
	ok()
	if !m.capturing {
		t.Error("OK on Snapshot must start capturing")
	}

	g.cfg.Bind(gb.ButtonA.String(), ebiten.KeyK)
	g.cfg.Screenshot = Hotkey{Key: ebiten.KeyF5}
	*m = menu{open: true, page: pageControls, cursor: controlsDefaults()}
	ok()
	if g.cfg.Key(gb.ButtonA) != ebiten.KeyX || g.cfg.Screenshot != defaultScreenshotHotkey() {
		t.Errorf("defaults: A=%s snapshot=%v", g.cfg.Key(gb.ButtonA), g.cfg.Screenshot)
	}
	if cfg, _ := LoadConfig(g.cfgPath); cfg.Key(gb.ButtonA) != ebiten.KeyX {
		t.Error("restored defaults not saved")
	}

	m.cursor = controlsBack()
	ok()
	if m.page != pageMain {
		t.Error("Back entry must return to the main page")
	}

	g.cfg.BindPad(gb.ButtonA.String(), std(ebiten.StandardGamepadButtonRightTop))
	*m = menu{open: true, page: pageControls, tab: tabPad, cursor: padDefaults()}
	ok()
	if g.cfg.PadButton(gb.ButtonA) != defaultPad()[gb.ButtonA.String()] {
		t.Errorf("pad defaults: A=%v", g.cfg.PadButton(gb.ButtonA))
	}
	m.cursor = padBack()
	ok()
	if m.page != pageMain || m.tab != tabKeyboard {
		t.Error("Back entry of the gamepad tab must return to the main page")
	}

	*m = menu{open: true, page: pageControls, tab: tabPad}
	g.actions = menuActions{left: true}
	m.update(g)
	if m.tab != tabKeyboard {
		t.Error("Left must go back to the keyboard tab")
	}
}

func TestMenuCaptureKey(t *testing.T) {
	g := newTestGame(t, newFakePads())
	m := &g.menu
	g.cfg.Screenshot = Hotkey{Key: ebiten.KeyF2}

	*m = menu{open: true, page: pageControls, cursor: 4, capturing: true} // A
	m.captureKey(g, Hotkey{Key: ebiten.KeyEscape})
	if m.capturing || g.cfg.Key(gb.ButtonA) != ebiten.KeyX {
		t.Error("Escape must cancel the capture")
	}

	m.capturing = true
	m.captureKey(g, Hotkey{Key: ebiten.KeyF2})
	if !m.capturing || m.notice != "F2 is already used by the snapshot" {
		t.Errorf("binding the bare snapshot key: capturing %v notice %q", m.capturing, m.notice)
	}
	m.captureKey(g, Hotkey{Key: ebiten.KeyK})
	if m.capturing || m.notice != "" || g.cfg.Key(gb.ButtonA) != ebiten.KeyK {
		t.Errorf("binding K: capturing %v notice %q A=%s", m.capturing, m.notice, g.cfg.Key(gb.ButtonA))
	}
	if cfg, _ := LoadConfig(g.cfgPath); cfg.Key(gb.ButtonA) != ebiten.KeyK {
		t.Error("new binding not saved")
	}

	*m = menu{open: true, page: pageControls, cursor: controlsScreenshot(), capturing: true}
	m.captureKey(g, Hotkey{Key: ebiten.KeyK})
	if !m.capturing || m.notice != "K is already used by a button" {
		t.Errorf("bare snapshot key on a button key: capturing %v notice %q", m.capturing, m.notice)
	}
	combo := Hotkey{Key: ebiten.KeyK, Control: true}
	m.captureKey(g, combo)
	if m.capturing || g.cfg.Screenshot != combo {
		t.Errorf("Ctrl+K: capturing %v snapshot %v", m.capturing, g.cfg.Screenshot)
	}
	m.capturing = true
	m.captureKey(g, Hotkey{Key: ebiten.KeyEscape, Shift: true}) // a combination, not a cancel
	if m.capturing || g.cfg.Screenshot.Key != ebiten.KeyEscape {
		t.Errorf("Shift+Esc: capturing %v snapshot %v", m.capturing, g.cfg.Screenshot)
	}
}

func TestMenuLines(t *testing.T) {
	pads := newFakePads()
	g := newTestGame(t, pads)
	m := &g.menu

	m.show()
	if _, _, footer := m.lines(g); footer != "P: palette   F11: full screen" {
		t.Errorf("footer without a gamepad: %q", footer)
	}

	*m = menu{open: true, page: pageControls, cursor: 4, capturing: true}
	v := m.view(g)
	if !strings.HasSuffix(v.items[4], "press a key...") || v.footer != "Esc: cancel" {
		t.Errorf("capturing A: %q / %q", v.items[4], v.footer)
	}
	m.cursor = controlsScreenshot()
	if v = m.view(g); !strings.HasSuffix(v.items[controlsScreenshot()], "press the combination...") {
		t.Errorf("capturing the snapshot: %q", v.items[controlsScreenshot()])
	}
	m.notice = "refused"
	if v = m.view(g); v.footer != "refused" {
		t.Errorf("notice not shown: %q", v.footer)
	}

	long := strings.Repeat("x", 40)
	pads.pads[0] = &fakePad{name: long, held: map[padButton]int{}, axes: map[ebiten.StandardGamepadAxis]float64{}}
	*m = menu{open: true}
	if _, _, footer := m.lines(g); footer != "P: palette  F11: full screen  Start+Select: menu" {
		t.Errorf("footer with a gamepad: %q", footer)
	}
	*m = menu{open: true, page: pageControls, tab: tabPad}
	v = m.view(g)
	if len(v.items) != padBack()+1 || v.footer != "Gamepad: "+long[:31]+"." {
		t.Errorf("gamepad tab: %d items, footer %q", len(v.items), v.footer)
	}
	if !v.tabs[1].active || v.tabs[1].disabled {
		t.Errorf("gamepad tab state: %+v", v.tabs[1])
	}
	m.capturing = true
	if v = m.view(g); !strings.HasSuffix(v.items[0], "press a button...") || v.footer != "Esc: cancel" {
		t.Errorf("capturing on the gamepad tab: %q / %q", v.items[0], v.footer)
	}
	m.notice = "refused"
	if v = m.view(g); v.footer != "refused" {
		t.Errorf("gamepad notice not shown: %q", v.footer)
	}
}

func TestSaveBattery(t *testing.T) {
	// LD A,0x0A; LD (0x0000),A; LD (0xA000),A; JR -2
	g := withGameBoy(t, newTestGame(t, newFakePads()),
		0x3E, 0x0A, 0xEA, 0x00, 0x00, 0xEA, 0x00, 0xA0, 0x18, 0xFE)

	g.saveBattery()
	if _, err := os.Stat(g.savePath); err == nil {
		t.Fatal("battery saved although the RAM is unchanged")
	}
	g.gb.RunFrame()
	g.saveBattery()
	data, err := os.ReadFile(g.savePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0x2000 || data[0] != 0x0A {
		t.Errorf("save is %d bytes starting with %02X", len(data), data[0])
	}
	if g.gb.Cart.Dirty() {
		t.Error("still dirty after saving")
	}
}

func TestIgnoreHeldInputs(t *testing.T) {
	pads := newFakePads("x")
	g := newTestGame(t, pads)
	a := g.cfg.PadButton(gb.ButtonA)
	pads.press(0, a)
	g.ignoreHeldInputs()
	if !g.ignoredPad[a] || len(g.ignoredPad) != 1 {
		t.Errorf("ignored pad buttons %v, want only %v", g.ignoredPad, a)
	}
	if out := padGameButtons(pads, g.cfg.Gamepad, g.ignoredPad); out[gb.ButtonA] {
		t.Error("an ignored button reached the game")
	}
}

func TestMenuActionsOr(t *testing.T) {
	a := menuActions{up: true, ok: true}.or(menuActions{left: true, toggle: true})
	if a != (menuActions{up: true, ok: true, left: true, toggle: true}) {
		t.Errorf("or: %+v", a)
	}
}
