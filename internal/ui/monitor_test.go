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
		g.actions = menuActions{right: true}
		g.menu.update(g)
		if mons.at != want {
			t.Errorf("Right: window on %d, want %d", mons.at, want)
		}
	}
	g.actions = menuActions{left: true} // wraps around
	g.menu.update(g)
	if cfg, _ := LoadConfig(g.cfgPath); mons.at != 1 || cfg.Monitor != "Apple TV" || cfg.MonitorIndex != 1 {
		t.Errorf("Left: window on %d, saved %q at %d", mons.at, cfg.Monitor, cfg.MonitorIndex)
	}
	g.actions = menuActions{ok: true} // OK cycles like Right
	g.menu.update(g)
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

func TestFullscreenEntry(t *testing.T) {
	g := newTestGame(t, newFakePads())
	defer ebiten.SetFullscreen(false)
	ebiten.SetFullscreen(false)
	if got := displayLabel(g, displayFullscreen); got != "Mode      < window >" {
		t.Errorf("entry %q", got)
	}
	g.menu = menu{open: true, page: pageDisplay, cursor: displayFullscreen}
	g.actions = menuActions{ok: true} // from a gamepad: A
	g.menu.update(g)
	if cfg, _ := LoadConfig(g.cfgPath); !ebiten.IsFullscreen() || !cfg.Fullscreen {
		t.Errorf("OK: fullscreen %v, saved %v", ebiten.IsFullscreen(), cfg.Fullscreen)
	}
	g.cfg.Language = "fr"
	if got := displayLabel(g, displayFullscreen); got != "Mode      < plein ecran >" {
		t.Errorf("French entry %q", got)
	}
	g.actions = menuActions{left: true}
	g.menu.update(g)
	if cfg, _ := LoadConfig(g.cfgPath); ebiten.IsFullscreen() || cfg.Fullscreen {
		t.Errorf("Left: fullscreen %v, saved %v", ebiten.IsFullscreen(), cfg.Fullscreen)
	}
}
