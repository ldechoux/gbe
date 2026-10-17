package ui

// The Link cable page of the menu: host a game, join one found on the local
// network, or unplug the cable.

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ldechoux/gbe/internal/link"
	"github.com/ldechoux/gbe/internal/menusound"
)

// Link cable page entries. Which are shown depends on the cable (see
// linkEntries).
const (
	linkHost = iota
	linkJoin
	linkCancel // stop hosting or connecting
	linkUnplug // while connected
	linkBack
)

// joinNameWidth is how many characters of a name and of a game title the
// list of the Join page shows; addrWidth, of the gbe connected to, whole
// when it is an address (255.255.255.255:65535).
const (
	joinNameWidth = 16
	addrWidth     = 21
)

// linkEntries lists the entries of the Link cable page: those to start a
// cable, or the one that stops it.
func linkEntries(g *Game) []int {
	switch {
	case g.link != nil:
		return []int{linkUnplug, linkBack}
	case g.cable != nil:
		return []int{linkCancel, linkBack}
	}
	return []int{linkHost, linkJoin, linkBack}
}

// showLink opens the Link cable page.
func (m *menu) showLink(g *Game) {
	g.stopBrowsing()
	m.page, m.cursor, m.notice, m.width = pageLink, linkEntries(g)[0], "", 0
}

// linkCursor is the entry the cursor is on, the first one when the entries
// changed under it (the cable connected, say).
func (m *menu) linkCursor(g *Game) int {
	entries := linkEntries(g)
	if !slices.Contains(entries, m.cursor) {
		m.cursor = entries[0]
	}
	return m.cursor
}

func (m *menu) moveLink(g *Game, delta int) {
	entries := linkEntries(g)
	i := slices.Index(entries, m.linkCursor(g))
	m.cursor = entries[(i+len(entries)+delta)%len(entries)]
}

// activateLink does what OK does on the selected entry.
func (m *menu) activateLink(g *Game) {
	switch m.linkCursor(g) {
	case linkHost:
		if err := g.startHosting(); err != nil {
			g.menuSound(menusound.Refuse)
			m.notice = g.tr().T("link.host_failed", err)
			return
		}
		g.menuSound(menusound.Enter)
		m.notice, m.cursor = "", linkCancel
	case linkJoin:
		g.menuSound(menusound.Enter)
		m.page, m.cursor, m.notice, m.width = pageJoin, 0, "", 0
		g.startBrowsing()
	case linkCancel, linkUnplug:
		g.menuSound(menusound.Change)
		g.stopCable()
		m.cursor = linkHost
	default:
		g.menuSound(menusound.Back)
		m.backToMain()
	}
}

// activateJoin joins the selected gbe, or goes back.
func (m *menu) activateJoin(g *Game) {
	found := g.foundGames()
	if m.cursor >= len(found) {
		g.menuSound(menusound.Back)
		m.showLink(g)
		return
	}
	g.menuSound(menusound.Enter)
	g.join(found[m.cursor])
	m.showLink(g)
}

// linkView is the Link cable page, with the state of the cable below.
func (m *menu) linkView(g *Game) menuView {
	l := g.tr()
	labels := map[int]string{
		linkHost:   l.T("link.host"),
		linkJoin:   l.T("link.join"),
		linkCancel: l.T("link.cancel"),
		linkUnplug: l.T("link.unplug"),
		linkBack:   l.T("link.back"),
	}
	v := menuView{title: l.T("link.title")}
	entries := linkEntries(g)
	for _, e := range entries {
		v.items = append(v.items, labels[e])
	}
	v.selected = slices.Index(entries, m.linkCursor(g))
	v.footer = linkFooter(g)
	if m.notice != "" {
		v.footer = m.notice + "\n" + l.T("link.help")
	}
	// The panel keeps the size of the longest entries and states.
	v.altItems = []string{labels[linkHost], labels[linkJoin], labels[linkCancel], labels[linkUnplug]}
	v.altFooters = []string{
		l.T("link.waiting") + "\n" + l.T("link.address", "000.000.000.000:00000"),
		l.T("link.joining", strings.Repeat("W", addrWidth)) + "\n" + l.T("link.help"),
		l.T("link.connected", strings.Repeat("W", joinNameWidth)) + "\n" + l.T("link.connected_help"),
	}
	return v
}

// linkFooter tells the state of the cable.
func linkFooter(g *Game) string {
	l := g.tr()
	switch {
	case g.link != nil:
		return l.T("link.connected", cut(g.cable.Peer(), joinNameWidth)) + "\n" + l.T("link.connected_help")
	case g.cable != nil && g.cable.Hosting():
		return l.T("link.waiting") + "\n" + l.T("link.address", hostAddress(g.cable.Port()))
	case g.cable != nil:
		target := g.linkTarget
		if target == "" { // -link-connect
			target = g.cable.Target()
		}
		return l.T("link.joining", cut(target, addrWidth)) + "\n" + l.T("link.help")
	}
	return l.T("link.help")
}

// hostAddress is the address the other gbe would connect to: the first one
// of this machine on a network, with the port unless it is the default one.
func hostAddress(port int) string {
	addr := "127.0.0.1"
	if addrs := link.LocalAddrs(); len(addrs) > 0 {
		addr = addrs[0]
	}
	if port != link.DefaultPort {
		addr = fmt.Sprintf("%s:%d", addr, port)
	}
	return addr
}

// joinView is the Join page: the gbe hosting on the local network, as they
// are heard.
func (m *menu) joinView(g *Game) menuView {
	l := g.tr()
	found := g.foundGames()
	m.cursor = min(m.cursor, len(found)) // a host may have left
	v := menuView{title: l.T("link.join_title"), selected: m.cursor}
	for _, f := range found {
		v.items = append(v.items, joinLabel(l.T("link.no_game"), f))
	}
	v.items = append(v.items, l.T("link.back"))
	switch {
	case g.browseErr != nil:
		v.footer = l.T("link.browse_failed", g.browseErr)
	case len(found) == 0:
		v.footer = l.T("link.searching") + "\n" + l.T("link.none")
	default:
		v.footer = l.T("link.searching")
	}
	wide := strings.Repeat("W", joinNameWidth)
	v.altItems = []string{joinLabel("", link.Found{Name: wide, Title: wide})}
	v.altFooters = []string{l.T("link.searching") + "\n" + l.T("link.none"), l.T("link.browse_failed", "address already in use")}
	v.rows = 5
	return v
}

// joinLabel names a gbe hosting: its name and its game.
func joinLabel(noGame string, f link.Found) string {
	title := f.Title
	if title == "" {
		title = noGame
	}
	return cut(f.Name, joinNameWidth) + " - " + cut(title, joinNameWidth)
}

// cut shortens s to n characters at most.
func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "."
	}
	return s
}
