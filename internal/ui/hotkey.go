package ui

import (
	"runtime"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Hotkey is a key combined with an exact set of modifiers, e.g. Cmd+F2.
type Hotkey struct {
	Key     ebiten.Key `json:"key"`
	Control bool       `json:"control,omitempty"`
	Alt     bool       `json:"alt,omitempty"`
	Shift   bool       `json:"shift,omitempty"`
	Meta    bool       `json:"meta,omitempty"` // Cmd on macOS
}

// defaultScreenshotHotkey is Cmd+F2 on macOS and Ctrl+F2 elsewhere.
func defaultScreenshotHotkey() Hotkey {
	if runtime.GOOS == "darwin" {
		return Hotkey{Key: ebiten.KeyF2, Meta: true}
	}
	return Hotkey{Key: ebiten.KeyF2, Control: true}
}

func isModifier(k ebiten.Key) bool {
	switch k {
	case ebiten.KeyControl, ebiten.KeyControlLeft, ebiten.KeyControlRight,
		ebiten.KeyAlt, ebiten.KeyAltLeft, ebiten.KeyAltRight,
		ebiten.KeyShift, ebiten.KeyShiftLeft, ebiten.KeyShiftRight,
		ebiten.KeyMeta, ebiten.KeyMetaLeft, ebiten.KeyMetaRight:
		return true
	}
	return false
}

// currentHotkey combines k with the modifiers currently held.
func currentHotkey(k ebiten.Key) Hotkey {
	return Hotkey{
		Key:     k,
		Control: ebiten.IsKeyPressed(ebiten.KeyControl),
		Alt:     ebiten.IsKeyPressed(ebiten.KeyAlt),
		Shift:   ebiten.IsKeyPressed(ebiten.KeyShift),
		Meta:    ebiten.IsKeyPressed(ebiten.KeyMeta),
	}
}

func (h Hotkey) hasModifiers() bool { return h.Control || h.Alt || h.Shift || h.Meta }

func (h Hotkey) valid() bool { return h.Key >= 0 && h.Key <= ebiten.KeyMax && !isModifier(h.Key) }

// justPressed reports whether the combination was triggered this frame. The
// modifiers must match exactly, so Cmd+F2 does not fire on Cmd+Shift+F2.
func (h Hotkey) justPressed() bool {
	return inpututil.IsKeyJustPressed(h.Key) && currentHotkey(h.Key) == h
}

func (h Hotkey) String() string {
	var parts []string
	if h.Control {
		parts = append(parts, "Ctrl")
	}
	if h.Alt {
		parts = append(parts, "Alt")
	}
	if h.Shift {
		parts = append(parts, "Shift")
	}
	if h.Meta {
		parts = append(parts, metaName())
	}
	return strings.Join(append(parts, keyLabel(h.Key)), "+")
}
