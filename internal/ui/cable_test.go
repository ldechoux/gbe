package ui

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/ldechoux/gbe/internal/gb"
)

// fakeCable is an end of the cable, connected by the test.
type fakeCable struct {
	links        chan *gb.Link
	hosting      bool
	target, peer string
	title        string
	closed       bool
}

func newFakeCable(hosting bool) *fakeCable {
	return &fakeCable{links: make(chan *gb.Link, 1), hosting: hosting, peer: "pc"}
}

func (c *fakeCable) Links() <-chan *gb.Link { return c.links }
func (c *fakeCable) Peer() string           { return c.peer }
func (c *fakeCable) SetTitle(title string)  { c.title = title }
func (c *fakeCable) Hosting() bool          { return c.hosting }
func (c *fakeCable) Port() int              { return 5555 }
func (c *fakeCable) Target() string         { return c.target }
func (c *fakeCable) Close()                 { c.closed = true }

// connect hands g a connection through its cable, returning the other end.
func connect(t *testing.T, g *Game, c *fakeCable) net.Conn {
	t.Helper()
	a, b := net.Pipe()
	l := gb.NewLink(a)
	t.Cleanup(func() { l.Close(); b.Close() })
	c.links <- l
	g.pollCable()
	if g.link != l || g.gb.Link() != l {
		t.Fatal("link cable not plugged in")
	}
	if g.toast != g.tr().T("toast.link_connected") {
		t.Errorf("toast %q", g.toast)
	}
	return b
}

func TestCable(t *testing.T) {
	g := withModes(withGameBoy(t, newTestGame(t, newFakePads())))
	g.started = true
	c := newFakeCable(true)
	g.cable = c
	g.pollCable() // nothing yet
	if g.link != nil {
		t.Fatal("plugged in without a connection")
	}
	other := connect(t, g, c)

	// What would break the step of the two Game Boys is refused, once per
	// press.
	if !g.linkBlocks(true) || g.toast != g.tr().T("toast.link_blocked") {
		t.Errorf("fast forward or rewind allowed while linked (toast %q)", g.toast)
	}
	g.toast = ""
	if !g.linkBlocks(true) || g.toast != "" {
		t.Error("the toast must show once per press")
	}
	if g.linkBlocks(false) {
		t.Error("blocked while not asked")
	}
	if err := os.WriteFile(g.statePath, g.gb.SaveState(), 0o644); err != nil {
		t.Fatal(err)
	}
	if g.loadState() {
		t.Error("state loaded while linked")
	}

	// A new console, in another mode, keeps the cable.
	g.cfg.ColorizeDMG = true
	old := g.gb
	g.restart()
	if g.gb == old || g.gb.Link() != g.link || old.Link() != nil {
		t.Error("the cable must move to the new console")
	}

	// Unplugged on the other side.
	other.Close()
	deadline := time.Now().Add(time.Second)
	for g.link != nil && time.Now().Before(deadline) {
		g.pollCable()
		time.Sleep(time.Millisecond)
	}
	if g.link != nil || g.gb.Link() != nil {
		t.Fatal("cable still plugged in after a disconnection")
	}
	if g.toast != g.tr().T("toast.link_disconnected") {
		t.Errorf("toast %q", g.toast)
	}
	if err := os.WriteFile(g.statePath, g.gb.SaveState(), 0o644); err != nil {
		t.Fatal(err)
	}
	if g.linkBlocks(true) || !g.loadState() {
		t.Error("still blocked once disconnected")
	}

	// Connected again.
	connect(t, g, c)
}
