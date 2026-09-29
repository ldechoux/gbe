package ui

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/gb"
)

// Config holds the user preferences persisted between runs.
type Config struct {
	Palette string                `json:"palette"`
	Scale   int                   `json:"scale"`
	Volume  float64               `json:"volume"`
	Keys    map[string]ebiten.Key `json:"keys"` // Game Boy button name -> key
	// Screenshot is the key combination that saves a PNG of the screen.
	Screenshot Hotkey `json:"screenshot"`
}

func defaultKeys() map[string]ebiten.Key {
	return map[string]ebiten.Key{
		gb.ButtonUp.String():     ebiten.KeyArrowUp,
		gb.ButtonDown.String():   ebiten.KeyArrowDown,
		gb.ButtonLeft.String():   ebiten.KeyArrowLeft,
		gb.ButtonRight.String():  ebiten.KeyArrowRight,
		gb.ButtonA.String():      ebiten.KeyX,
		gb.ButtonB.String():      ebiten.KeyZ,
		gb.ButtonStart.String():  ebiten.KeyEnter,
		gb.ButtonSelect.String(): ebiten.KeyShiftRight,
	}
}

// DefaultConfig returns the out-of-the-box settings.
func DefaultConfig() *Config {
	return &Config{
		Palette:    Palettes[0].ID,
		Scale:      4,
		Volume:     0.8,
		Keys:       defaultKeys(),
		Screenshot: defaultScreenshotHotkey(),
	}
}

// DefaultConfigPath is <user config dir>/gbe/config.json.
func DefaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "gbe-config.json"
	}
	return filepath.Join(dir, "gbe", "config.json")
}

// LoadConfig reads the config at path. A missing file yields the defaults;
// missing or invalid entries are replaced by their default value.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	var loaded struct {
		Config
		Screenshot *Hotkey `json:"screenshot"` // nil when absent
	}
	if err := json.Unmarshal(data, &loaded); err != nil {
		return cfg, err
	}
	if loaded.Screenshot != nil && loaded.Screenshot.valid() {
		cfg.Screenshot = *loaded.Screenshot
	}
	if loaded.Palette != "" {
		cfg.Palette = Palettes[paletteIndex(loaded.Palette)].ID
	}
	if loaded.Scale >= 1 && loaded.Scale <= maxScale {
		cfg.Scale = loaded.Scale
	}
	if loaded.Volume >= 0 && loaded.Volume <= 1 {
		cfg.Volume = loaded.Volume
	}
	for name, key := range loaded.Keys {
		if _, ok := cfg.Keys[name]; ok {
			cfg.Keys[name] = key
		}
	}
	return cfg, nil
}

// Save writes the config to path, creating the directory if needed.
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Key returns the key bound to a Game Boy button.
func (c *Config) Key(b gb.Button) ebiten.Key { return c.Keys[b.String()] }

// Bind assigns key to button. If another button used that key, it gets the
// button's previous key, so no two buttons ever share a key.
func (c *Config) Bind(b gb.Button, key ebiten.Key) {
	old := c.Keys[b.String()]
	for name, k := range c.Keys {
		if k == key {
			c.Keys[name] = old
		}
	}
	c.Keys[b.String()] = key
}

// conflicts reports whether a game button bound to key would also fire the
// screenshot hotkey (only possible when the hotkey has no modifier).
func (c *Config) conflicts(key ebiten.Key) bool {
	return !c.Screenshot.hasModifiers() && c.Screenshot.Key == key
}

// bound reports whether key is assigned to a button.
func (c *Config) bound(key ebiten.Key) bool {
	for _, k := range c.Keys {
		if k == key {
			return true
		}
	}
	return false
}
