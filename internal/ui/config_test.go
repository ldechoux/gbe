package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/gb"
)

func TestLoadConfigIgnoresInvalidEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{
		"palette": "nope",
		"filter": "nope",
		"scale": 99,
		"volume": 2,
		"keys": {"A": "K", "Turbo": "L"},
		"gamepad": {"A": "NoSuchButton", "Turbo": "`+padButtonNames[std(ebiten.StandardGamepadButtonRightTop)]+`"},
		"screenshot": {"key": "ShiftLeft"}
	}`), 0o644)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	def := DefaultConfig()
	if cfg.Palette != def.Palette || cfg.Filter != def.Filter || cfg.Scale != def.Scale || cfg.Volume != def.Volume {
		t.Errorf("invalid values kept: palette %q filter %q scale %d volume %v", cfg.Palette, cfg.Filter, cfg.Scale, cfg.Volume)
	}
	if cfg.Key(gb.ButtonA) != ebiten.KeyK || len(cfg.Keys) != len(def.Keys) {
		t.Errorf("keys %v: want A rebound and Turbo dropped", cfg.Keys)
	}
	if cfg.PadButton(gb.ButtonA) != def.PadButton(gb.ButtonA) || len(cfg.Gamepad) != len(def.Gamepad) {
		t.Errorf("gamepad %v: want the defaults", cfg.Gamepad)
	}
	if cfg.Screenshot != def.Screenshot {
		t.Errorf("invalid snapshot hotkey kept: %v", cfg.Screenshot)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(filepath.Join(dir, "missing.json"))
	if err != nil || cfg.Scale != DefaultConfig().Scale {
		t.Errorf("missing file: %v, %+v", err, cfg)
	}

	path := filepath.Join(dir, "broken.json")
	os.WriteFile(path, []byte(`{"scale": `), 0o644)
	cfg, err = LoadConfig(path)
	if err == nil || cfg == nil || cfg.Scale != DefaultConfig().Scale {
		t.Errorf("broken file: %v, %+v; want an error and the defaults", err, cfg)
	}

	cfg, err = LoadConfig(dir) // a directory cannot be read
	if err == nil || cfg == nil {
		t.Errorf("unreadable file: %v, %+v; want an error and the defaults", err, cfg)
	}
}

func TestSaveConfigErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	os.WriteFile(file, nil, 0o644)
	if err := DefaultConfig().Save(filepath.Join(file, "config.json")); err == nil {
		t.Error("saving under a regular file succeeded")
	}
}

func TestConfigBound(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.bound(ebiten.KeyX) {
		t.Error("X is bound to A by default")
	}
	if cfg.bound(ebiten.KeyF2) {
		t.Error("F2 is not bound to any button")
	}
}

func TestDefaultPaths(t *testing.T) {
	if p := DefaultConfigPath(); filepath.Base(p) != "config.json" && p != "gbe-config.json" {
		t.Errorf("DefaultConfigPath() = %q", p)
	}
	if d := DefaultScreenshotDir(); d == "" {
		t.Error("DefaultScreenshotDir() is empty")
	}
}

func TestAudioStreamOverflow(t *testing.T) {
	s := &audioStream{}
	s.push(make([]int16, 2*100))
	if got := s.buffered(); got != 100 {
		t.Errorf("buffered %d frames, want 100", got)
	}
	s.push(make([]int16, 2*maxFill))
	if got := s.buffered(); got != targetFill {
		t.Errorf("after overflow: %d frames, want %d", got, targetFill)
	}
}
