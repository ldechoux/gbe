package ui

import (
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
}

func (a menuActions) or(b menuActions) menuActions {
	return menuActions{
		up: a.up || b.up, down: a.down || b.down, left: a.left || b.left, right: a.right || b.right,
		ok: a.ok || b.ok, back: a.back || b.back, toggle: a.toggle || b.toggle,
	}
}

// repeatTick is true when a control is first pressed, then periodically
// while it is held. d is the number of ticks it has been held.
func repeatTick(d int) bool { return d == 1 || (d > 20 && d%5 == 0) }

func pressed(k ebiten.Key) bool { return inpututil.IsKeyJustPressed(k) }

func repeated(k ebiten.Key) bool { return repeatTick(inpututil.KeyPressDuration(k)) }

func keyboardActions() menuActions {
	return menuActions{
		up:    repeated(ebiten.KeyArrowUp),
		down:  repeated(ebiten.KeyArrowDown),
		left:  repeated(ebiten.KeyArrowLeft),
		right: repeated(ebiten.KeyArrowRight),
		ok:    pressed(ebiten.KeyEnter) || pressed(ebiten.KeyNumpadEnter) || pressed(ebiten.KeySpace),
		back:  pressed(ebiten.KeyEscape),
	}
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
	start, sel := dur(gb.ButtonStart), dur(gb.ButtonSelect)
	a := menuActions{
		up:     repeatTick(dur(gb.ButtonUp)),
		down:   repeatTick(dur(gb.ButtonDown)),
		left:   repeatTick(dur(gb.ButtonLeft)),
		right:  repeatTick(dur(gb.ButtonRight)),
		ok:     dur(gb.ButtonA) == 1,
		back:   dur(gb.ButtonB) == 1,
		toggle: start > 0 && sel > 0 && (start == 1 || sel == 1),
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
	a.left = a.left || repeatTick(st.held[2])
	a.right = a.right || repeatTick(st.held[3])
	return a
}
