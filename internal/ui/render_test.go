package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/rendertest"
	"github.com/ldechoux/gbe/internal/rom"
)

// The rendering tests draw the screens of gbe as the player sees them, and
// compare them with reference images (see internal/rendertest):
//
//	GBE_RENDER=1 go test ./internal/ui
func TestMain(m *testing.M) { rendertest.Main(m) }

// renderGame is a Game ready to draw, in English, without a ROM.
func renderGame(t *testing.T, pads padReader) *Game {
	t.Helper()
	rendertest.Skip(t)
	g := newTestGame(t, pads)
	g.cfg.Language = "en"
	// The default is Cmd+F2 on macOS and Ctrl+F2 elsewhere: the same
	// everywhere, for the same image.
	g.cfg.Screenshot = Hotkey{Key: ebiten.KeyF2, Control: true}
	// The keys are named after the layout of the keyboard (Z is W on an
	// AZERTY one): the same names everywhere, those of QWERTY.
	name := keyName
	keyName = func(ebiten.Key) string { return "" }
	t.Cleanup(func() { keyName = name })
	g.lcd = ebiten.NewImage(gb.ScreenWidth, gb.ScreenHeight)
	g.lcdPrev = ebiten.NewImage(gb.ScreenWidth, gb.ScreenHeight)
	g.stream = &audioStream{}
	return g
}

// withBenchROM runs the game scene of the benchmark ROM (benchrom/) on
// model, for 120 frames.
func withBenchROM(t *testing.T, g *Game, model gb.Model) *Game {
	t.Helper()
	game, err := rom.Open("../../benchrom/gbe-bench.gbc", rom.Options{Model: model, BootROM: "none", Fresh: true})
	if err != nil {
		t.Fatal(err)
	}
	g.gb, g.title, g.romPath = game.Console, game.Title, game.Path
	g.statePath, g.savePath = t.TempDir()+"/bench.state", t.TempDir()+"/bench.sav"
	g.started = true
	for f := range 120 {
		g.gb.SetButton(gb.ButtonA, f < 10) // the game scene
		g.gb.RunFrame()
	}
	g.gb.APU.DrainSamples()
	return g
}

// draw draws the window of g, w x h pixels, as Ebitengine would.
func draw(g *Game, w, h int) *ebiten.Image {
	dst := ebiten.NewImage(w, h)
	g.Draw(dst)
	return dst
}

func TestRenderDropScreen(t *testing.T) {
	g := renderGame(t, newFakePads())
	rendertest.Check(t, "drop_empty", draw(g, 320, 288))

	// Recent games, with long names, and All games, selected.
	g.cfg.Recent = []RecentGame{
		{Path: "/roms/Legend of Zelda, The - Link's Awakening DX (France) (Rev 2).gbc"},
		{Path: "/roms/Super Mario Land (World) (Rev 1).gb"},
		{Path: "/roms/Tetris (World).gb"},
	}
	for i := range 12 {
		g.library.games = append(g.library.games, libraryGame{path: fmt.Sprintf("/roms/g%d.gb", i), name: fmt.Sprint(i)})
	}
	g.recentCursor = 3
	rendertest.Check(t, "drop_recent", draw(g, 640, 576))

	// The first recent game selected: its name scrolls, from its start.
	g.recentCursor = 0
	rendertest.Check(t, "drop_recent_selected", draw(g, 640, 576))

	// The smallest window: only the selected entry fits.
	rendertest.Check(t, "drop_small", draw(g, 160, 144))
}

func TestRenderMenus(t *testing.T) {
	g := withBenchROM(t, renderGame(t, newFakePads("Xbox Wireless Controller")), gb.ModelDMG)
	g.cfg.Filter = "nearest"
	g.cfg.Recent = []RecentGame{{Path: g.romPath}}
	m := &g.menu
	scenes := []struct {
		name string
		set  func()
	}{
		{"game", func() { m.open = false }},
		{"pause", func() { m.show() }},
		{"display", func() { *m = menu{open: true, page: pageDisplay, cursor: displayFilter} }},
		{"sound", func() { *m = menu{open: true, page: pageSound, cursor: soundStereo} }},
		{"controls_keyboard", func() { *m = menu{open: true, page: pageControls, cursor: 2} }},
		{"controls_pad", func() {
			g.cfg.Vibration = vibrationAll
			*m = menu{open: true, page: pageControls, tab: tabPad, cursor: padStrength()}
		}},
		{"controls_capture", func() { *m = menu{open: true, page: pageControls, cursor: 1, capturing: true} }},
		{"games_recent", func() { *m = menu{open: true, page: pageGames, tab: tabRecent} }},
		{"folders_boot_rom", func() { *m = menu{open: true, page: pageFolders, tab: tabBootROM, cursor: bootChoose} }},
		{"start", func() {
			g.stateTime = time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC)
			m.showStart()
		}},
	}
	for _, s := range scenes {
		s.set()
		rendertest.Check(t, s.name, draw(g, 640, 576))
	}
}

// The long list of the games folder: the scroll bar, the names cut, and the
// selected one in the middle.
func TestRenderGamesList(t *testing.T) {
	g := renderGame(t, newFakePads())
	g.cfg.GamesDir = "/Users/me/Games"
	for i := range 60 {
		name := fmt.Sprintf("Game %02d", i)
		if i%7 == 0 {
			name += " - A very long name that does not fit (Europe) (En,Fr,De)"
		}
		sub := ""
		if i%3 == 0 {
			sub = "GBC"
		}
		g.library.games = append(g.library.games, libraryGame{path: fmt.Sprintf("/g/%02d.gb", i), name: name, sub: sub})
	}
	g.menu = menu{open: true, page: pageGames, tab: tabAll, cursor: 21}
	rendertest.Check(t, "games_all", draw(g, 640, 576))
	g.menu.cursor = 0
	rendertest.Check(t, "games_all_top", draw(g, 320, 288))
	g.menu = menu{open: true, page: pageFolders, tab: tabGamesDir}
	rendertest.Check(t, "folders_games", draw(g, 640, 576))
}

// The menu of the smallest windows: compact, the entries scrolled.
func TestRenderSmallWindow(t *testing.T) {
	g := withBenchROM(t, renderGame(t, newFakePads()), gb.ModelDMG)
	g.menu = menu{open: true, page: pageMain, cursor: itemQuit}
	rendertest.Check(t, "pause_small", draw(g, 160, 144))
}

// A Game Boy Color game, its colors corrected, under the LCD filter, with
// the speed badge and a notification.
func TestRenderColorGame(t *testing.T) {
	g := withBenchROM(t, renderGame(t, newFakePads()), gb.ModelCGB)
	g.cfg.Filter, g.cfg.ColorCorrection = "lcd", true
	g.speed = playFast
	g.notify("State saved")
	rendertest.Check(t, "color_lcd", draw(g, 640, 576))

	// The screenshot hotkey saves the same frame, without the badge.
	g.speed = playNormal
	img := ebiten.NewImageFromImage(g.renderFrame(2))
	rendertest.Check(t, "color_screenshot", img)
}

// The Camera page of a Game Boy Camera game, a picture chosen.
func TestRenderCamera(t *testing.T) {
	g := renderGame(t, newFakePads())
	rom := make([]byte, 0x8000)
	copy(rom[0x134:], "GAMEBOYCAMERA")
	rom[0x147], rom[0x149] = 0xFC, 0x04
	copy(rom[0x100:], []byte{0x18, 0xFE}) // JR -2
	cart, err := gb.NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	if g.gb, err = gb.New(cart, nil); err != nil {
		t.Fatal(err)
	}
	g.cfg.CameraImage, g.cfg.CameraMirror = "/Users/me/Pictures/selfie.png", true
	g.menu = menu{open: true, page: pageMain, cursor: itemCamera}
	rendertest.Check(t, "camera_pause", draw(g, 640, 576))
	g.menu = menu{open: true, page: pageCamera, cursor: cameraMirror}
	rendertest.Check(t, "camera", draw(g, 640, 576))
}
