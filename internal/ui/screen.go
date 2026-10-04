package ui

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/gb"
)

// screenSwitchTicks is how long gbe leaves the window alone after asking
// for the full screen or the window. On macOS, the switch is an animation of
// about half a second: another switch requested meanwhile is ignored, or
// crashes AppKit ("Must have content controller"), and resizing the window
// leaves it in a mixed state. Keys released during the animation may also
// never be seen released (see staleKeys).
const screenSwitchTicks = 60

// snapshotTicks is how long the full screen waits after it was asked for:
// macOS animates the switch with the last frame drawn, which must already
// show the new mode in the menu, or the old one comes back during the
// animation.
const snapshotTicks = 2

// now is the current tick, replaced in tests.
var now = ebiten.Tick

// fullscreen reports the mode of the window, or, while it switches, the
// mode it switches to.
func (g *Game) fullscreen() bool {
	if g.screenBusy() {
		return g.cfg.Fullscreen
	}
	return ebiten.IsFullscreen()
}

// screenBusy reports whether the window may still be switching.
func (g *Game) screenBusy() bool { return now() < g.screenBusyUntil }

// switchingScreen notes a switch asked for now, which the next switches
// wait for.
func (g *Game) switchingScreen() {
	g.screenBusyUntil = now() + screenSwitchTicks
	g.screenCheck = true
	g.markStaleKeys()
}

// markStaleKeys sets aside the keys held while the window switches: macOS
// may lose their release, and they would then look held forever, the menu
// repeating their action (an arrow on the Mode entry switching again and
// again). The menu and the game ignore them until they are seen released.
func (g *Game) markStaleKeys() {
	for _, k := range pressedKeys() {
		if g.staleKeys == nil {
			g.staleKeys = map[ebiten.Key]bool{}
		}
		g.staleKeys[k] = true
		if g.ignoredKeys != nil {
			g.ignoredKeys[k] = true // for the game, until released too
		}
	}
}

// forgetReleasedKeys keeps the stale keys that are still held. While the
// window switches, the keys pressed meanwhile become stale too.
func (g *Game) forgetReleasedKeys() {
	if g.screenBusy() {
		g.markStaleKeys()
	}
	for k := range g.staleKeys {
		if keyDuration(k) == 0 {
			delete(g.staleKeys, k)
		}
	}
}

// toggleFullscreen switches between the window and the full screen, which
// the next launch restores, snapshotTicks later (see settleScreen). It does
// nothing while a switch is under way.
func (g *Game) toggleFullscreen() {
	if g.screenBusy() {
		return
	}
	g.cfg.Fullscreen = !ebiten.IsFullscreen()
	g.fullscreenAt, g.fullscreenPending = now()+snapshotTicks, true
	g.switchingScreen()
	g.screenBusyUntil += snapshotTicks
	g.saveConfig()
}

// applyScale sizes the window for the scale of the settings. In the full
// screen, or while switching, the window waits to be back (settleScreen).
func (g *Game) applyScale() {
	if g.screenBusy() || g.fullscreen() {
		g.sizePending = true
		return
	}
	g.sizePending = false
	ebiten.SetWindowSize(gb.ScreenWidth*g.cfg.Scale, gb.ScreenHeight*g.cfg.Scale)
}

// settleScreen runs once the window is done switching: it applies the
// scale chosen meanwhile, and keeps the setting in line with the actual
// mode, which macOS may not have switched to (a request it ignored, or its
// green button).
func (g *Game) settleScreen() {
	if g.fullscreenPending && now() >= g.fullscreenAt {
		g.fullscreenPending = false
		ebiten.SetFullscreen(g.cfg.Fullscreen)
	}
	g.forgetReleasedKeys()
	if g.screenBusy() || (!g.screenCheck && !g.sizePending) {
		return
	}
	g.screenCheck = false
	if full := ebiten.IsFullscreen(); full != g.cfg.Fullscreen {
		g.cfg.Fullscreen = full
		g.saveConfig()
	}
	if g.sizePending && !g.cfg.Fullscreen {
		g.applyScale()
	}
}
