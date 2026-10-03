package ui

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/i18n"
	"github.com/ldechoux/gbe/internal/ui/scaler"
)

// Config holds the user preferences persisted between runs.
type Config struct {
	Palette string `json:"palette"`
	// ColorCorrection makes Game Boy Color games look like on the real
	// screen instead of showing their raw, oversaturated colors.
	ColorCorrection bool `json:"color_correction"`
	// Language is the code of the user interface language (see i18n).
	Language string `json:"language"`
	Scale    int    `json:"scale"`
	// Filter draws the picture on the screen (see scaler.Filters).
	Filter string `json:"filter"`
	// Ghosting blends each frame with the previous one, like the LCD.
	Ghosting   bool                  `json:"ghosting"`
	Fullscreen bool                  `json:"fullscreen"`
	Volume     float64               `json:"volume"`
	Keys       map[string]ebiten.Key `json:"keys"` // button or action name -> key
	// Gamepad maps button and action names to standard layout gamepad buttons.
	Gamepad map[string]padButton `json:"gamepad"`
	// Vibration makes the gamepads shake with the motor of rumble
	// cartridges (Pokemon Pinball...).
	Vibration bool `json:"vibration"`
	// Screenshot is the key combination that saves a PNG of the screen.
	Screenshot Hotkey `json:"screenshot"`
	// FastForwardSpeed is how many frames run per frame while fast forwarding.
	FastForwardSpeed int `json:"fast_forward_speed"`
	// ColorizeDMG runs the DMG games on a Game Boy Color, which colorizes
	// them, instead of their original hardware (when the model is left to
	// auto). Off by default.
	ColorizeDMG bool `json:"colorize_dmg"`
	// CompatPalettes is the palette chosen for a colorized DMG game, by
	// title (see compatPaletteID). Absent: the one the boot ROM picks.
	CompatPalettes map[string]string `json:"compat_palettes"`
}

// Emulator actions held like the Game Boy buttons, bound in Keys and
// Gamepad next to them.
const (
	actionFastForward = "FastForward"
	actionRewind      = "Rewind"
)

const (
	minFastForward = 2
	maxFastForward = 8
)

// bindingNames lists the Game Boy buttons, then the actions, in the order of
// the controls page.
func bindingNames() []string {
	names := make([]string, 0, len(gb.Buttons)+2)
	for _, b := range gb.Buttons {
		names = append(names, b.String())
	}
	return append(names, actionFastForward, actionRewind)
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
		actionFastForward:        ebiten.KeyTab,
		actionRewind:             ebiten.KeyBackspace,
	}
}

// DefaultConfig returns the out-of-the-box settings.
func DefaultConfig() *Config {
	return &Config{
		Palette:          Palettes[0].ID,
		ColorCorrection:  true,
		Vibration:        true,
		Language:         i18n.Default,
		Scale:            4,
		Filter:           scaler.Filters[0].ID,
		Volume:           0.8,
		Keys:             defaultKeys(),
		Gamepad:          defaultPad(),
		Screenshot:       defaultScreenshotHotkey(),
		FastForwardSpeed: 4,
		CompatPalettes:   map[string]string{},
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
		Screenshot      *Hotkey `json:"screenshot"`       // nil when absent
		ColorCorrection *bool   `json:"color_correction"` // same
		ColorizeDMG     *bool   `json:"colorize_dmg"`     // same
		Vibration       *bool   `json:"vibration"`        // same
	}
	if err := json.Unmarshal(data, &loaded); err != nil {
		return cfg, err
	}
	if loaded.Screenshot != nil && loaded.Screenshot.valid() {
		cfg.Screenshot = *loaded.Screenshot
	}
	if loaded.ColorCorrection != nil {
		cfg.ColorCorrection = *loaded.ColorCorrection
	}
	if loaded.ColorizeDMG != nil {
		cfg.ColorizeDMG = *loaded.ColorizeDMG
	}
	if loaded.Vibration != nil {
		cfg.Vibration = *loaded.Vibration
	}
	cfg.Ghosting, cfg.Fullscreen = loaded.Ghosting, loaded.Fullscreen
	for title, id := range loaded.CompatPalettes {
		if compatPaletteIndex(id) != gb.CompatAuto {
			cfg.CompatPalettes[title] = id
		}
	}
	if loaded.Palette != "" {
		cfg.Palette = Palettes[paletteIndex(loaded.Palette)].ID
	}
	if i18n.Has(loaded.Language) {
		cfg.Language = loaded.Language
	}
	if id, ok := scaler.ID(loaded.Filter); ok {
		cfg.Filter = id
	}
	if loaded.Scale >= 1 && loaded.Scale <= maxScale {
		cfg.Scale = loaded.Scale
	}
	if loaded.Volume >= 0 && loaded.Volume <= 1 {
		cfg.Volume = loaded.Volume
	}
	if loaded.FastForwardSpeed >= minFastForward && loaded.FastForwardSpeed <= maxFastForward {
		cfg.FastForwardSpeed = loaded.FastForwardSpeed
	}
	for name, btn := range loaded.Gamepad {
		if _, ok := cfg.Gamepad[name]; ok && btn != padNone {
			cfg.Gamepad[name] = btn
		}
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

// Bind assigns key to a button or action (see bindingNames). If another one
// used that key, it gets the previous key, so no two ever share a key.
func (c *Config) Bind(name string, key ebiten.Key) {
	old := c.Keys[name]
	for n, k := range c.Keys {
		if k == key {
			c.Keys[n] = old
		}
	}
	c.Keys[name] = key
}

// BindPad assigns a gamepad button to a button or action, swapping with the
// one that used it, like Bind.
func (c *Config) BindPad(name string, btn padButton) {
	old := c.Gamepad[name]
	for n, pb := range c.Gamepad {
		if pb == btn {
			c.Gamepad[n] = old
		}
	}
	c.Gamepad[name] = btn
}

// PadButton returns the gamepad button bound to a Game Boy button.
func (c *Config) PadButton(b gb.Button) padButton { return c.Gamepad[b.String()] }

// conflicts reports whether a button or action bound to key would also fire the
// screenshot hotkey (only possible when the hotkey has no modifier).
func (c *Config) conflicts(key ebiten.Key) bool {
	return !c.Screenshot.hasModifiers() && c.Screenshot.Key == key
}

// bound reports whether key is assigned to a button or an action.
func (c *Config) bound(key ebiten.Key) bool {
	for _, k := range c.Keys {
		if k == key {
			return true
		}
	}
	return false
}
