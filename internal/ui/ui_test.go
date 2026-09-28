package ui

import (
	"encoding/binary"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"gbe/internal/gb"
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
	cases := map[ebiten.Key]string{
		ebiten.KeyQ:          "Q",
		ebiten.KeyArrowUp:    "Fleche haut",
		ebiten.KeyShiftRight: "Maj droite",
		ebiten.KeyNumpad1:    "Pave 1",
		ebiten.KeyF2:         "F2",
	}
	for k, want := range cases {
		if got := keyLabel(k); got != want {
			t.Errorf("keyLabel(%s) = %q, want %q", k, got, want)
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
