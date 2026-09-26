package ui

import (
	"encoding/binary"
	"path/filepath"
	"testing"

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
