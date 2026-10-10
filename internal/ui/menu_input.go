package ui

import (
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/ldechoux/gbe/internal/gb"
)

// menuActions is what the menu reacts to, whether it comes from the keyboard
// or a gamepad. Directions auto-repeat while held.
type menuActions struct {
	up, down, left, right bool
	ok, back              bool
	toggle                bool // Start+Select on a gamepad: open or close the menu
	// The long lists also go a page at a time (L and R on a gamepad), to
	// the next name starting with a letter typed, and faster the longer Up
	// or Down is held: for how many ticks they have been.
	pageUp, pageDown bool
	letter           rune // 0 for none
	upHeld, downHeld int
}

func (a menuActions) or(b menuActions) menuActions {
	return menuActions{
		up: a.up || b.up, down: a.down || b.down, left: a.left || b.left, right: a.right || b.right,
		ok: a.ok || b.ok, back: a.back || b.back, toggle: a.toggle || b.toggle,
		pageUp: a.pageUp || b.pageUp, pageDown: a.pageDown || b.pageDown, letter: max(a.letter, b.letter),
		upHeld: max(a.upHeld, b.upHeld), downHeld: max(a.downHeld, b.downHeld),
	}
}

// repeatTick is true when a control is first pressed, then periodically
// while it is held. d is the number of ticks it has been held.
func repeatTick(d int) bool { return d == 1 || (d > 20 && d%5 == 0) }

// fastRepeatTick is repeatTick for the long lists: 12 times a second, as
// repeatTick, then 30 after a second held, then 60 after two.
func fastRepeatTick(d int) bool {
	switch {
	case d > 120:
		return true
	case d > 60:
		return d%2 == 0
	}
	return repeatTick(d)
}

// letterKeys are the keys a name may start with, which the long lists jump
// to.
var letterKeys = func() []ebiten.Key {
	var keys []ebiten.Key
	for k := ebiten.KeyA; k <= ebiten.KeyZ; k++ {
		keys = append(keys, k)
	}
	for k := ebiten.KeyDigit0; k <= ebiten.KeyDigit9; k++ {
		keys = append(keys, k)
	}
	return keys
}()

// keyName is ebiten.KeyName, replaced in tests.
var keyName = ebiten.KeyName

// keyLetter is the letter or digit of k on the active layout (the A key of
// a QWERTY keyboard is Q on an AZERTY one), 0 for none.
func keyLetter(k ebiten.Key) rune {
	if r := []rune(keyName(k)); len(r) == 1 && (unicode.IsLetter(r[0]) || unicode.IsDigit(r[0])) {
		return unicode.ToLower(r[0])
	}
	switch {
	case k >= ebiten.KeyDigit0 && k <= ebiten.KeyDigit9: // AZERTY: the digits are shifted
		return '0' + rune(k-ebiten.KeyDigit0)
	case k >= ebiten.KeyA && k <= ebiten.KeyZ: // no name yet (tests)
		return 'a' + rune(k-ebiten.KeyA)
	}
	return 0
}

// The keyboard state, replaced in tests.
var (
	keyDuration = inpututil.KeyPressDuration
	pressedKeys = func() []ebiten.Key { return inpututil.AppendPressedKeys(nil) }
)

func pressed(k ebiten.Key) bool { return inpututil.IsKeyJustPressed(k) }

// keyboardActions reads the menu actions of the keyboard. The stale keys
// (see Game.staleKeys) do not count: they may be held only because their
// release was lost.
func keyboardActions(stale map[ebiten.Key]bool) menuActions {
	held := func(k ebiten.Key) int {
		if stale[k] {
			return 0
		}
		return keyDuration(k)
	}
	repeated := func(k ebiten.Key) bool { return repeatTick(held(k)) }
	a := menuActions{
		up:       repeated(ebiten.KeyArrowUp),
		down:     repeated(ebiten.KeyArrowDown),
		left:     repeated(ebiten.KeyArrowLeft),
		right:    repeated(ebiten.KeyArrowRight),
		ok:       pressed(ebiten.KeyEnter) || pressed(ebiten.KeyNumpadEnter) || pressed(ebiten.KeySpace),
		back:     pressed(ebiten.KeyEscape),
		pageUp:   repeated(ebiten.KeyPageUp),
		pageDown: repeated(ebiten.KeyPageDown),
		upHeld:   held(ebiten.KeyArrowUp),
		downHeld: held(ebiten.KeyArrowDown),
	}
	for _, k := range letterKeys {
		if held(k) == 1 {
			a.letter = keyLetter(k)
			break
		}
	}
	return a
}

// stickNav tracks how long the left stick has pointed up, down, left and
// right, so it repeats in the menu like a held D-pad.
type stickNav struct{ held [4]int }

// padActions maps the gamepad onto menu actions using the player's own
// mapping: the Game Boy D-pad navigates, A validates, B goes back, and
// Start+Select toggles the menu. It must be called once per tick.
func padActions(r padReader, mapping map[string]padButton, st *stickNav) menuActions {
	dur := func(b gb.Button) int {
		d := 0
		for _, id := range r.ids() {
			d = max(d, r.duration(id, mapping[b.String()]))
		}
		return d
	}
	// The shoulder buttons, which a Game Boy does not have: whatever the
	// mapping.
	shoulder := func(b ebiten.StandardGamepadButton) int {
		d := 0
		for _, id := range r.ids() {
			d = max(d, r.duration(id, padButton(b)))
		}
		return d
	}
	start, sel := dur(gb.ButtonStart), dur(gb.ButtonSelect)
	a := menuActions{
		up:       repeatTick(dur(gb.ButtonUp)),
		down:     repeatTick(dur(gb.ButtonDown)),
		left:     repeatTick(dur(gb.ButtonLeft)),
		right:    repeatTick(dur(gb.ButtonRight)),
		ok:       dur(gb.ButtonA) == 1,
		back:     dur(gb.ButtonB) == 1,
		toggle:   start > 0 && sel > 0 && (start == 1 || sel == 1),
		pageUp:   repeatTick(shoulder(ebiten.StandardGamepadButtonFrontTopLeft)),
		pageDown: repeatTick(shoulder(ebiten.StandardGamepadButtonFrontTopRight)),
		upHeld:   dur(gb.ButtonUp),
		downHeld: dur(gb.ButtonDown),
	}

	var dirs [4]bool
	for _, id := range r.ids() {
		u, d, l, rt := stickDirs(r, id)
		dirs = [4]bool{dirs[0] || u, dirs[1] || d, dirs[2] || l, dirs[3] || rt}
	}
	for i, on := range dirs {
		if on {
			st.held[i]++
		} else {
			st.held[i] = 0
		}
	}
	a.up = a.up || repeatTick(st.held[0])
	a.down = a.down || repeatTick(st.held[1])
	a.upHeld, a.downHeld = max(a.upHeld, st.held[0]), max(a.downHeld, st.held[1])
	a.left = a.left || repeatTick(st.held[2])
	a.right = a.right || repeatTick(st.held[3])
	return a
}
