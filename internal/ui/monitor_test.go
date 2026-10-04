package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/i18n"
)

// fakeMonitors is a monitorSet whose window is on monitor at.
type fakeMonitors struct {
	list []string
	at   int
}

func (f *fakeMonitors) names() []string { return f.list }
func (f *fakeMonitors) current() int    { return f.at }
func (f *fakeMonitors) use(i int)       { f.at = i }

func TestFindMonitor(t *testing.T) {
	names := []string{"Generic PnP Monitor", "Generic PnP Monitor", "Apple TV"}
	for _, tc := range []struct {
		name  string
		index int
		want  int
	}{
		{"Apple TV", 2, 2},
		{"Apple TV", 0, 2},            // moved: found by name
		{"Generic PnP Monitor", 1, 1}, // same names: the index decides
		{"Generic PnP Monitor", 2, 0}, // the first one with that name
		{"Generic PnP Monitor", 7, 0}, // index out of range
		{"Living room TV", 1, -1},     // unplugged
	} {
		if got := findMonitor(names, tc.name, tc.index); got != tc.want {
			t.Errorf("findMonitor(%q, %d) = %d, want %d", tc.name, tc.index, got, tc.want)
		}
	}
}

func TestRestoreMonitor(t *testing.T) {
	g := newTestGame(t, newFakePads())
	mons := &fakeMonitors{list: []string{"Built-in Retina Display", "Apple TV"}}
	g.monitors = mons

	g.restoreMonitor()
	if mons.at != 0 {
		t.Errorf("no monitor saved: window on %d, want the primary one", mons.at)
	}
	g.cfg.Monitor, g.cfg.MonitorIndex = "Apple TV", 1
	g.restoreMonitor()
	if mons.at != 1 {
		t.Errorf("window on %d, want the Apple TV", mons.at)
	}
	mons.at, mons.list = 0, mons.list[:1] // the TV is off
	g.restoreMonitor()
	if mons.at != 0 {
		t.Errorf("TV gone: window on %d, want the primary one", mons.at)
	}
}

func TestMonitorEntry(t *testing.T) {
	g := newTestGame(t, newFakePads())
	if displayLabel(g, displayMonitor) != "" {
		t.Error("entry shown without monitors")
	}
	mons := &fakeMonitors{list: []string{"Built-in Retina Display"}}
	g.monitors = mons
	if displayLabel(g, displayMonitor) != "" {
		t.Error("entry shown with a single monitor")
	}

	mons.list = append(mons.list, "Apple TV")
	if got := displayLabel(g, displayMonitor); got != "Monitor   < 1 - Built-in Retina Display >" {
		t.Errorf("entry %q", got)
	}
	g.menu = menu{open: true, page: pageDisplay, cursor: displayMonitor}
	for _, want := range []int{1, 0} {
		g.screenBusyUntil = 0 // the previous move is over (see screen.go)
		g.actions = menuActions{right: true}
		g.menu.update(g)
		if mons.at != want {
			t.Errorf("Right: window on %d, want %d", mons.at, want)
		}
	}
	g.screenBusyUntil = 0
	g.actions = menuActions{left: true} // wraps around
	g.menu.update(g)
	if cfg, _ := LoadConfig(g.cfgPath); mons.at != 1 || cfg.Monitor != "Apple TV" || cfg.MonitorIndex != 1 {
		t.Errorf("Left: window on %d, saved %q at %d", mons.at, cfg.Monitor, cfg.MonitorIndex)
	}
	g.actions = menuActions{ok: true} // too soon: the window is moving
	g.menu.update(g)
	if mons.at != 1 {
		t.Errorf("OK while moving: window on %d, want 1", mons.at)
	}
	g.screenBusyUntil = 0
	g.menu.update(g) // OK cycles like Right
	if mons.at != 0 {
		t.Errorf("OK: window on %d, want 0", mons.at)
	}

	mons.at = 1
	g.cfg.Language = "fr"
	if got := displayLabel(g, displayMonitor); got != "Ecran     < 2 - Apple TV >" {
		t.Errorf("French entry %q", got)
	}
	mons.list[1] = "A monitor with a very long name indeed"
	if got := monitorEntry(i18n.Get("en"), g); got != "Monitor   < 2 - A monitor with a very... >" {
		t.Errorf("long name %q", got)
	}
}

// fakeClock replaces the tick of screen.go until the test ends.
func fakeClock(t *testing.T) *int64 {
	var tick int64
	prev := now
	now = func() int64 { return tick }
	t.Cleanup(func() { now = prev })
	return &tick
}

// wait lets the ticks of a switch pass, as Update does.
func wait(g *Game, tick *int64, ticks int64) {
	for range ticks {
		*tick++
		g.settleScreen()
	}
}

func TestFullscreenEntry(t *testing.T) {
	tick := fakeClock(t)
	g := newTestGame(t, newFakePads())
	defer ebiten.SetFullscreen(false)
	ebiten.SetFullscreen(false)
	if got := displayLabel(g, displayFullscreen); got != "Mode      < window >" {
		t.Errorf("entry %q", got)
	}
	g.menu = menu{open: true, page: pageDisplay, cursor: displayFullscreen}
	g.actions = menuActions{ok: true} // from a gamepad: A
	g.menu.update(g)
	// The menu shows the new mode at once, before macOS animates the
	// switch with the last frame drawn.
	if cfg, _ := LoadConfig(g.cfgPath); ebiten.IsFullscreen() || !cfg.Fullscreen ||
		displayLabel(g, displayFullscreen) != "Mode      < fullscreen >" {
		t.Errorf("OK: fullscreen %v, saved %v, entry %q", ebiten.IsFullscreen(), cfg.Fullscreen, displayLabel(g, displayFullscreen))
	}
	wait(g, tick, snapshotTicks)
	if !ebiten.IsFullscreen() {
		t.Errorf("not in the full screen %d ticks later", snapshotTicks)
	}
	g.cfg.Language = "fr"
	if got := displayLabel(g, displayFullscreen); got != "Mode      < plein ecran >" {
		t.Errorf("French entry %q", got)
	}

	g.actions = menuActions{left: true} // too soon: still switching
	g.menu.update(g)
	wait(g, tick, snapshotTicks)
	if !ebiten.IsFullscreen() || !g.cfg.Fullscreen {
		t.Error("switched back during the switch")
	}
	wait(g, tick, screenSwitchTicks) // the switch is over
	g.menu.update(g)
	wait(g, tick, snapshotTicks)
	if cfg, _ := LoadConfig(g.cfgPath); ebiten.IsFullscreen() || cfg.Fullscreen {
		t.Errorf("Left: fullscreen %v, saved %v", ebiten.IsFullscreen(), cfg.Fullscreen)
	}
}

// The scale chosen in the full screen, or while switching, applies once
// back in the window; the setting follows the mode the window ended in.
func TestSettleScreen(t *testing.T) {
	tick := fakeClock(t)
	g := newTestGame(t, newFakePads())
	defer ebiten.SetFullscreen(false)
	ebiten.SetFullscreen(false)
	ebiten.SetWindowSize(160, 144)

	g.toggleFullscreen() // to the full screen
	g.cfg.Scale = 3
	g.applyScale()
	wait(g, tick, snapshotTicks)
	if !g.sizePending || !g.screenCheck {
		t.Fatal("resized or settled while switching")
	}

	// macOS ignored the request: the setting follows the actual mode.
	ebiten.SetFullscreen(false)
	wait(g, tick, screenSwitchTicks)
	if cfg, _ := LoadConfig(g.cfgPath); g.cfg.Fullscreen || cfg.Fullscreen {
		t.Error("the setting says full screen, the window is not")
	}
	if g.sizePending || g.screenCheck {
		t.Error("the scale chosen meanwhile is not applied")
	}
	if w, h := ebiten.WindowSize(); w != 480 || h != 432 {
		t.Errorf("window %dx%d, want 480x432", w, h)
	}

	// In the full screen, the scale waits for the window.
	g.toggleFullscreen()
	wait(g, tick, snapshotTicks+screenSwitchTicks)
	g.cfg.Scale = 2
	g.applyScale()
	g.settleScreen()
	if !g.sizePending {
		t.Error("resized in the full screen")
	}
}

// An arrow released during a switch may never be seen released: held
// forever, it would repeat its action (Mode switching again and again).
// It is ignored until it is seen released.
func TestStaleKeysAfterSwitch(t *testing.T) {
	held := map[ebiten.Key]int{}
	defer func(d func(ebiten.Key) int, p func() []ebiten.Key) { keyDuration, pressedKeys = d, p }(keyDuration, pressedKeys)
	keyDuration = func(k ebiten.Key) int { return held[k] }
	pressedKeys = func() []ebiten.Key {
		var ks []ebiten.Key
		for k, d := range held {
			if d > 0 {
				ks = append(ks, k)
			}
		}
		return ks
	}
	defer ebiten.SetFullscreen(false)

	g := newTestGame(t, newFakePads())
	held[ebiten.KeyArrowRight] = 1 // Right on Mode
	if !keyboardActions(g.staleKeys).right {
		t.Fatal("Right not read")
	}
	g.toggleFullscreen()
	for d := 2; d < 200; d++ { // its release is lost: held for good
		held[ebiten.KeyArrowRight] = d
		g.forgetReleasedKeys()
		if keyboardActions(g.staleKeys).right {
			t.Fatalf("the stale Right repeats at tick %d", d)
		}
	}
	if !g.ignoredKeys[ebiten.KeyArrowRight] {
		t.Error("the game would see the stale Right held")
	}

	// A key pressed during the switch is stale too.
	held[ebiten.KeyArrowDown] = 1
	g.forgetReleasedKeys()
	if keyboardActions(g.staleKeys).down {
		t.Error("Down pressed during the switch counts")
	}

	// Once seen released, then pressed again, Right works again.
	g.screenBusyUntil = 0
	held[ebiten.KeyArrowRight], held[ebiten.KeyArrowDown] = 0, 0
	g.forgetReleasedKeys()
	held[ebiten.KeyArrowRight] = 1
	if !keyboardActions(g.staleKeys).right || len(g.staleKeys) != 0 {
		t.Errorf("Right after its release: stale %v", g.staleKeys)
	}
}
