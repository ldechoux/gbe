package ui

import (
	"runtime"
	"strings"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
)

// Bindings are stored as physical keys (named after their US QWERTY
// position, e.g. KeyZ), which keeps them valid whatever the layout. Only the
// labels shown to the user follow the active keyboard layout.

// specialKeyLabels names the keys that have no character of their own.
var specialKeyLabels = map[ebiten.Key]string{
	ebiten.KeyArrowUp:      "Fleche haut",
	ebiten.KeyArrowDown:    "Fleche bas",
	ebiten.KeyArrowLeft:    "Fleche gauche",
	ebiten.KeyArrowRight:   "Fleche droite",
	ebiten.KeyEnter:        "Entree",
	ebiten.KeyNumpadEnter:  "Entree (pave)",
	ebiten.KeySpace:        "Espace",
	ebiten.KeyTab:          "Tab",
	ebiten.KeyBackspace:    "Retour arriere",
	ebiten.KeyEscape:       "Echap",
	ebiten.KeyDelete:       "Suppr",
	ebiten.KeyInsert:       "Inser",
	ebiten.KeyHome:         "Debut",
	ebiten.KeyEnd:          "Fin",
	ebiten.KeyPageUp:       "Page haut",
	ebiten.KeyPageDown:     "Page bas",
	ebiten.KeyCapsLock:     "Verr. Maj",
	ebiten.KeyShiftLeft:    "Maj gauche",
	ebiten.KeyShiftRight:   "Maj droite",
	ebiten.KeyControlLeft:  "Ctrl gauche",
	ebiten.KeyControlRight: "Ctrl droite",
	ebiten.KeyAltLeft:      "Alt gauche",
	ebiten.KeyAltRight:     "Alt droite",
	ebiten.KeyMetaLeft:     metaName() + " gauche",
	ebiten.KeyMetaRight:    metaName() + " droite",
}

func metaName() string {
	if runtime.GOOS == "darwin" {
		return "Cmd"
	}
	return "Meta"
}

// keyLabel returns how the physical key k is labeled on the user's keyboard,
// e.g. KeyQ is "A" on an AZERTY layout.
func keyLabel(k ebiten.Key) string {
	name := k.String()
	// Keypad digits would be confused with the digits of the main row.
	if rest, ok := strings.CutPrefix(name, "Numpad"); ok {
		return "Pave " + rest
	}
	// KeyName follows the active layout. It is empty for keys without a
	// character, and before the main loop starts (e.g. in tests).
	if n := ebiten.KeyName(k); printable(n) {
		if len(n) == 1 && n[0] >= 'a' && n[0] <= 'z' {
			n = strings.ToUpper(n) // letters as printed on the keycaps
		}
		return n
	}
	if l, ok := specialKeyLabels[k]; ok {
		return l
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
