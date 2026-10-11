package ui

import (
	"errors"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/gb"
)

// A tick of Update, driven by the gamepad: it runs the game with the
// buttons held, opens and closes the menu, fast forwards and rewinds, and
// stops once the player quits.
func TestUpdate(t *testing.T) {
	pads := newFakePads("Pad")
	g := withGameBoy(t, newTestGame(t, pads), frameProgram...)
	g.stream = &audioStream{}
	btn := func(name string) padButton { return g.cfg.Gamepad[name] }
	tick := func() {
		t.Helper()
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
		pads.tick()
	}

	// A held: the game sees it, and a frame runs.
	pads.press(0, btn(gb.ButtonA.String()))
	cycles := g.gb.Cycles()
	tick()
	if !g.down[4] || g.frame != 1 || g.gb.Cycles() == cycles || g.speed != playNormal {
		t.Fatalf("a frame with A: down %v, frame %d, speed %v", g.down, g.frame, g.speed)
	}
	pads.release(0, btn(gb.ButtonA.String()))

	// R held: fast forward, then L: rewind.
	pads.press(0, btn(actionFastForward))
	tick()
	if g.speed != playFast {
		t.Errorf("R held: speed %v", g.speed)
	}
	pads.release(0, btn(actionFastForward))
	pads.press(0, btn(actionRewind))
	tick()
	tick()
	if g.speed != playRewind {
		t.Errorf("L held: speed %v", g.speed)
	}
	pads.release(0, btn(actionRewind))

	// Start+Select opens the menu; the game waits while it is open.
	pads.press(0, btn(gb.ButtonStart.String()))
	pads.press(0, btn(gb.ButtonSelect.String()))
	tick()
	if !g.menu.open {
		t.Fatal("Start+Select must open the menu")
	}
	pads.release(0, btn(gb.ButtonStart.String()))
	pads.release(0, btn(gb.ButtonSelect.String()))
	frame := g.frame
	tick()
	if g.frame != frame {
		t.Error("the game ran with the menu open")
	}
	// B closes it; the buttons still held then are ignored until released.
	pads.press(0, btn(gb.ButtonB.String()))
	tick()
	if g.menu.open || !g.ignoredPad[btn(gb.ButtonB.String())] {
		t.Fatalf("B: menu open %v, ignored %v", g.menu.open, g.ignoredPad)
	}
	tick()
	if g.down[5] {
		t.Error("B, held when the menu closed, reached the game")
	}
	pads.release(0, btn(gb.ButtonB.String()))
	tick()
	if len(g.ignoredPad) != 0 {
		t.Errorf("released, B is still ignored: %v", g.ignoredPad)
	}

	// The battery save is written every saveEveryFrame frames.
	g.frame = saveEveryFrame - 1
	if err := g.gb.Restore(g.gb.Snapshot(nil)); err != nil { // the cartridge RAM must reach the .sav
		t.Fatal(err)
	}
	tick()
	if _, err := os.Stat(g.savePath); err != nil {
		t.Errorf("the battery save was not written: %v", err)
	}

	g.quit = true
	if err := g.Update(); !errors.Is(err, ebiten.Termination) {
		t.Errorf("quit: %v", err)
	}
}

// Without a game, Update drives the drop screen.
func TestUpdateDropScreen(t *testing.T) {
	pads := newFakePads("Pad")
	g := newTestGame(t, pads)
	g.cfg.Recent = []RecentGame{{Path: "/r/a.gb"}, {Path: "/r/b.gb"}}
	pads.press(0, g.cfg.Gamepad[gb.ButtonDown.String()])
	if err := g.Update(); err != nil || g.recentCursor != 1 {
		t.Errorf("Down: %v, cursor %d", err, g.recentCursor)
	}
}
