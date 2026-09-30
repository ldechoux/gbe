package ui

import (
	"runtime"
	"strings"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/i18n"
)

// Bindings are stored as physical keys (named after their US QWERTY
// position, e.g. KeyZ), which keeps them valid whatever the layout. Only the
// labels shown to the user follow the active keyboard layout.

// specialKeyLabels gives the message keys naming the keys that have no
// character of their own.
var specialKeyLabels = map[ebiten.Key]string{
	ebiten.KeyArrowUp:      "key.arrow_up",
	ebiten.KeyArrowDown:    "key.arrow_down",
	ebiten.KeyArrowLeft:    "key.arrow_left",
	ebiten.KeyArrowRight:   "key.arrow_right",
	ebiten.KeyEnter:        "key.enter",
	ebiten.KeyNumpadEnter:  "key.numpad_enter",
	ebiten.KeySpace:        "key.space",
	ebiten.KeyTab:          "key.tab",
	ebiten.KeyBackspace:    "key.backspace",
	ebiten.KeyEscape:       "key.escape",
	ebiten.KeyDelete:       "key.delete",
	ebiten.KeyInsert:       "key.insert",
	ebiten.KeyHome:         "key.home",
	ebiten.KeyEnd:          "key.end",
	ebiten.KeyPageUp:       "key.page_up",
	ebiten.KeyPageDown:     "key.page_down",
	ebiten.KeyCapsLock:     "key.caps_lock",
	ebiten.KeyShiftLeft:    "key.shift_left",
	ebiten.KeyShiftRight:   "key.shift_right",
	ebiten.KeyControlLeft:  "key.control_left",
	ebiten.KeyControlRight: "key.control_right",
	ebiten.KeyAltLeft:      "key.alt_left",
	ebiten.KeyAltRight:     "key.alt_right",
	ebiten.KeyMetaLeft:     "key.meta_left",  // takes metaName()
	ebiten.KeyMetaRight:    "key.meta_right", // takes metaName()
}

func metaName() string {
	if runtime.GOOS == "darwin" {
		return "Cmd"
	}
	return "Meta"
}

// keyLabel returns how the physical key k is labeled on the user's keyboard,
// e.g. KeyQ is "A" on an AZERTY layout, in language l.
func keyLabel(l *i18n.Locale, k ebiten.Key) string {
	name := k.String()
	// Keypad digits would be confused with the digits of the main row.
	if rest, ok := strings.CutPrefix(name, "Numpad"); ok {
		return l.T("key.numpad", rest)
	}
	// KeyName follows the active layout. It is empty for keys without a
	// character, and before the main loop starts (e.g. in tests).
	if n := ebiten.KeyName(k); printable(n) {
		if len(n) == 1 && n[0] >= 'a' && n[0] <= 'z' {
			n = strings.ToUpper(n) // letters as printed on the keycaps
		}
		return n
	}
	switch key, ok := specialKeyLabels[k]; {
	case k == ebiten.KeyMetaLeft || k == ebiten.KeyMetaRight:
		return l.T(key, metaName())
	case ok:
		return l.T(key)
	}
	return name
}

// printable rejects empty names and whitespace ones (Space, Tab), which would
// show as a blank label.
func printable(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
