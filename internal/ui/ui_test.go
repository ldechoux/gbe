package ui

import (
	"encoding/binary"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/i18n"
)

func TestConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	cfg := DefaultConfig()
	cfg.Palette = "amber"
	cfg.Scale = 3
	cfg.Volume = 0.5
	cfg.Bind(gb.ButtonA, ebiten.KeyK)
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Palette != "amber" || got.Scale != 3 || got.Volume != 0.5 {
		t.Fatalf("loaded %+v", got)
	}
	for _, b := range gb.Buttons {
		if got.Key(b) != cfg.Key(b) {
			t.Errorf("%s bound to %s, want %s", b, got.Key(b), cfg.Key(b))
		}
	}
}

func TestBindSwapsConflicts(t *testing.T) {
	cfg := DefaultConfig()
	oldA := cfg.Key(gb.ButtonA)
	cfg.Bind(gb.ButtonA, cfg.Key(gb.ButtonB))
	if cfg.Key(gb.ButtonA) != ebiten.KeyZ || cfg.Key(gb.ButtonB) != oldA {
		t.Fatalf("A=%s B=%s", cfg.Key(gb.ButtonA), cfg.Key(gb.ButtonB))
	}
}

func TestAudioStreamUnderrun(t *testing.T) {
	s := &audioStream{}
	s.push([]int16{100, -100})
	p := make([]byte, 12) // 3 frames, only 1 available
	if n, _ := s.Read(p); n != 12 {
		t.Fatalf("read %d bytes", n)
	}
	for i := range 3 {
		l := int16(binary.LittleEndian.Uint16(p[i*4:]))
		r := int16(binary.LittleEndian.Uint16(p[i*4+2:]))
		if l != 100 || r != -100 {
			t.Fatalf("frame %d = %d,%d; want last frame repeated", i, l, r)
		}
	}
}

func TestEmulatedRateTracksFill(t *testing.T) {
	low, high := emulatedRate(0), emulatedRate(maxFill)
	if !(low > emulatedRate(targetFill) && emulatedRate(targetFill) > high) {
		t.Fatalf("rate should decrease as the buffer fills: %v %v", low, high)
	}
}

func TestScreenshotHotkeyPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := DefaultConfig()
	cfg.Screenshot = Hotkey{Key: ebiten.KeyS, Control: true, Shift: true}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Screenshot != cfg.Screenshot {
		t.Fatalf("loaded %+v, want %+v", got.Screenshot, cfg.Screenshot)
	}
	if s := got.Screenshot.String(); s != "Ctrl+Shift+S" {
		t.Fatalf("String() = %q", s)
	}

	// A config written before the feature existed gets the default hotkey.
	if err := os.WriteFile(path, []byte(`{"palette":"dmg"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ = LoadConfig(path)
	if got.Screenshot != defaultScreenshotHotkey() {
		t.Fatalf("missing hotkey loaded as %+v", got.Screenshot)
	}
}

func TestScreenshotConflicts(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.conflicts(ebiten.KeyF2) {
		t.Fatal("a hotkey with a modifier never conflicts with a button")
	}
	cfg.Screenshot = Hotkey{Key: ebiten.KeyF2}
	if !cfg.conflicts(ebiten.KeyF2) {
		t.Fatal("a bare F2 hotkey conflicts with a button on F2")
	}
}

func TestSaveScreenshot(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shots")
	var fb [gb.ScreenWidth * gb.ScreenHeight]byte
	fb[0] = 3
	img := ScreenshotImage(&fb, &Palettes[0], 2)
	if b := img.Bounds(); b.Dx() != 320 || b.Dy() != 288 {
		t.Fatalf("image size %v", b)
	}
	if img.RGBAAt(1, 1) != Palettes[0].Colors[3] || img.RGBAAt(2, 0) != Palettes[0].Colors[0] {
		t.Fatal("pixels not scaled with the palette")
	}
	now := time.Date(2026, 9, 27, 14, 3, 5, 0, time.Local)
	p1, err := saveScreenshot(dir, "SUPER MARIOLAND", img, now)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := saveScreenshot(dir, "SUPER MARIOLAND", img, now)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p1) != "super-marioland-20260927-140305.png" ||
		filepath.Base(p2) != "super-marioland-20260927-140305-2.png" {
		t.Fatalf("names %s, %s", p1, p2)
	}
	f, err := os.Open(p1)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := png.Decode(f); err != nil {
		t.Fatal(err)
	}
}

// Without a running game loop ebiten.KeyName is empty, which exercises the
// fallbacks (the layout-aware path needs a window).
func TestKeyLabelFallbacks(t *testing.T) {
	cases := map[string]map[ebiten.Key]string{
		"fr": {
			ebiten.KeyQ:          "Q",
			ebiten.KeyArrowUp:    "Fleche haut",
			ebiten.KeyShiftRight: "Maj droite",
			ebiten.KeyNumpad1:    "Pave 1",
			ebiten.KeyF2:         "F2",
		},
		"en": {
			ebiten.KeyArrowUp:   "Up arrow",
			ebiten.KeyMetaLeft:  "Left " + metaName(),
			ebiten.KeyNumpad1:   "Keypad 1",
			ebiten.KeyBackspace: "Backspace",
		},
	}
	for lang, labels := range cases {
		for k, want := range labels {
			if got := keyLabel(i18n.Get(lang), k); got != want {
				t.Errorf("%s: keyLabel(%s) = %q, want %q", lang, k, got, want)
			}
		}
	}
	for k, key := range specialKeyLabels {
		if !i18n.Get(i18n.Default).Has(key) {
			t.Errorf("%s: no message %q", k, key)
		}
	}
}

func TestPrintable(t *testing.T) {
	for s, want := range map[string]bool{"a": true, "é": true, "^": true, "": false, " ": false, "\t": false} {
		if printable(s) != want {
			t.Errorf("printable(%q) = %v", s, !want)
		}
	}
}

func TestFPSCounter(t *testing.T) {
	var c fpsCounter
	start := time.Unix(1000, 0)
	if c.update(start) {
		t.Fatal("the first call only starts the interval")
	}
	for range 30 {
		c.frame()
	}
	if c.update(start.Add(fpsRefreshInterval / 2)) {
		t.Fatal("measured before the interval elapsed")
	}
	if !c.update(start.Add(fpsRefreshInterval)) {
		t.Fatal("no measure once the interval elapsed")
	}
	want := 30 / fpsRefreshInterval.Seconds()
	if c.fps != want {
		t.Fatalf("fps = %v, want %v", c.fps, want)
	}
	// A new interval starts from zero.
	if !c.update(start.Add(2*fpsRefreshInterval)) || c.fps != 0 {
		t.Fatalf("fps = %v after an interval without frames, want 0", c.fps)
	}
}

func TestWindowTitle(t *testing.T) {
	cases := []struct {
		rom    string
		fps    float64
		paused bool
		want   string
	}{
		{"SUPER MARIOLAND", 59.73, false, "gbe - SUPER MARIOLAND - 60 FPS"},
		{"SUPER MARIOLAND", 0, true, "gbe - SUPER MARIOLAND - Paused"},
		{"", 30, false, "gbe - 30 FPS"},
	}
	for _, c := range cases {
		if got := windowTitle(i18n.Get("en"), c.rom, c.fps, c.paused); got != c.want {
			t.Errorf("windowTitle(%q, %v, %v) = %q, want %q", c.rom, c.fps, c.paused, got, c.want)
		}
	}
}

func TestConfigLanguage(t *testing.T) {
	dir := t.TempDir()
	for content, want := range map[string]string{
		`{}`:                "en", // English when no language was chosen
		`{"language":"fr"}`: "fr",
		`{"language":"xx"}`: "en", // unknown language
	} {
		path := filepath.Join(dir, "c.json")
		os.WriteFile(path, []byte(content), 0o644)
		cfg, err := LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Language != want {
			t.Errorf("%s: language %q, want %q", content, cfg.Language, want)
		}
	}
}

func TestMenuLanguage(t *testing.T) {
	g := newTestGame(t, newFakePads())
	g.menu = menu{open: true, cursor: itemLanguage}
	if title, items, _ := g.menu.lines(g); title != "PAUSE" || items[itemQuit] != "Quit" {
		t.Fatalf("default menu: %q %q", title, items)
	}
	g.actions = menuActions{right: true}
	g.menu.update(g)
	if _, items, _ := g.menu.lines(g); items[itemLanguage] != "Langue    < Francais >" || items[itemQuit] != "Quitter" {
		t.Fatalf("French menu: %q", items)
	}
	cfg, err := LoadConfig(g.cfgPath)
	if err != nil || cfg.Language != "fr" {
		t.Fatalf("saved language %q (%v)", cfg.Language, err)
	}
	g.actions = menuActions{left: true}
	g.menu.update(g)
	if g.cfg.Language != "en" {
		t.Fatalf("language %q after Left", g.cfg.Language)
	}
}
