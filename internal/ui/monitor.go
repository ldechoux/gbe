package ui

import (
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/i18n"
)

// monitorNameMax is the longest monitor name shown in the menu.
const monitorNameMax = 24

// monitorSet abstracts the monitors so the logic can be tested without them.
type monitorSet interface {
	// names lists the connected monitors, the primary one first. The list
	// changes when one is plugged in, e.g. an Apple TV over AirPlay.
	names() []string
	// current is the index of the monitor the window is on.
	current() int
	// use moves the window, or the full screen, to monitor i.
	use(i int)
}

type ebitenMonitors struct{ buf []*ebiten.MonitorType }

func (m *ebitenMonitors) list() []*ebiten.MonitorType {
	m.buf = ebiten.AppendMonitors(m.buf[:0])
	return m.buf
}

func (m *ebitenMonitors) names() []string {
	var names []string
	for _, mon := range m.list() {
		names = append(names, mon.Name())
	}
	return names
}

func (m *ebitenMonitors) current() int {
	mons, cur := m.list(), ebiten.Monitor()
	if i := slices.Index(mons, cur); i >= 0 || cur == nil {
		return max(0, i)
	}
	// The list is rebuilt when a monitor is plugged in or out.
	return max(0, slices.IndexFunc(mons, func(mon *ebiten.MonitorType) bool { return mon.Name() == cur.Name() }))
}

func (m *ebitenMonitors) use(i int) {
	if mons := m.list(); i >= 0 && i < len(mons) {
		ebiten.SetMonitor(mons[i])
	}
}

// findMonitor is the index of the monitor named name, -1 if it is gone. On
// Windows, every monitor has the same name: index, where it was, tells
// them apart.
func findMonitor(names []string, name string, index int) int {
	if index >= 0 && index < len(names) && names[index] == name {
		return index
	}
	return slices.Index(names, name)
}

// multiMonitor reports whether there is a monitor to choose from, which
// shows the Monitor entry of the display page.
func (g *Game) multiMonitor() bool {
	return g.monitors != nil && len(g.monitors.names()) > 1
}

// restoreMonitor opens the window on the monitor of the last run, if it is
// still connected, or else leaves it on the primary one.
func (g *Game) restoreMonitor() {
	if g.cfg.Monitor == "" {
		return
	}
	if i := findMonitor(g.monitors.names(), g.cfg.Monitor, g.cfg.MonitorIndex); i >= 0 {
		g.monitors.use(i)
	}
}

// cycleMonitor moves the window to the previous (-1) or next (1) monitor,
// which the next launch restores.
func (g *Game) cycleMonitor(delta int) {
	names := g.monitors.names()
	n := len(names)
	if n == 0 {
		return
	}
	i := (g.monitors.current() + delta%n + n) % n
	g.monitors.use(i)
	g.cfg.Monitor, g.cfg.MonitorIndex = names[i], i
	g.saveConfig()
}

// monitorEntry is the Monitor entry of the display page: the number of the
// monitor the window is on, from 1, and its name, shortened.
func monitorEntry(l *i18n.Locale, g *Game) string {
	if !g.multiMonitor() {
		return "" // hidden
	}
	names := g.monitors.names()
	i := min(g.monitors.current(), len(names)-1)
	name := []rune(names[i])
	if len(name) > monitorNameMax {
		name = append(name[:monitorNameMax-3], []rune("...")...)
	}
	return l.T("menu.monitor", i+1, string(name))
}
