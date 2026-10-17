package ui

import (
	"fmt"
	"log"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/link"
)

// CableEnd is an end of the link cable (see link.Cable): each connection
// made comes through Links.
type CableEnd interface {
	Links() <-chan *gb.Link
	Peer() string // the name of the other gbe, while connected
	SetTitle(title string)
	Hosting() bool
	Port() int      // the one Hosting listens on
	Target() string // the address it connects to, unless Hosting
	Close()
}

// gameBrowser lists the gbe hosting on the local network (see
// link.Browser).
type gameBrowser interface {
	Games() []link.Found
	Close()
}

// cableMakers make the ends of the cable and the browser, for the menu:
// link.Host, link.Connect and link.Browse, except in tests.
type cableMakers struct {
	host   func() (CableEnd, error)
	join   func(addr string) CableEnd
	browse func() (gameBrowser, error)
}

// systemCable makes the cable over the network, on the default port.
var systemCable = cableMakers{
	host: func() (CableEnd, error) {
		c, err := link.Host(fmt.Sprintf(":%d", link.DefaultPort), link.LocalName())
		if err != nil {
			return nil, err
		}
		return c, nil
	},
	join: func(addr string) CableEnd { return link.Connect(addr, link.LocalName()) },
	browse: func() (gameBrowser, error) {
		b, err := link.Browse()
		if err != nil {
			return nil, err
		}
		return b, nil
	},
}

// startHosting waits for another gbe, in place of the cable in use.
func (g *Game) startHosting() error {
	c, err := g.cables.host()
	if err != nil {
		log.Printf("link cable: %v", err)
		return err
	}
	g.setCable(c, "")
	log.Printf("link cable: waiting on port %d", c.Port())
	return nil
}

// join connects to the gbe found, in place of the cable in use.
func (g *Game) join(f link.Found) {
	g.setCable(g.cables.join(f.Addr), f.Name)
	log.Printf("link cable: connecting to %s (%s)", f.Name, f.Addr)
}

// setCable puts c in place of the cable in use; target names the gbe it
// connects to.
func (g *Game) setCable(c CableEnd, target string) {
	g.stopCable()
	g.cable, g.linkTarget = c, target
	if g.gb != nil {
		c.SetTitle(g.title)
	}
}

// stopCable stops hosting or connecting, and unplugs the cable.
func (g *Game) stopCable() {
	if g.cable == nil {
		return
	}
	g.cable.Close()
	g.cable, g.linkTarget = nil, ""
	if g.link != nil {
		g.link = nil
		if g.gb != nil {
			g.gb.SetLink(nil)
		}
		log.Print("link cable unplugged")
	}
}

// startBrowsing looks for the gbe hosting on the local network, for the
// Join page.
func (g *Game) startBrowsing() {
	g.stopBrowsing()
	g.browser, g.browseErr = g.cables.browse()
	if g.browseErr != nil {
		log.Printf("link cable: %v", g.browseErr)
	}
}

// stopBrowsing stops looking, once the Join page is left.
func (g *Game) stopBrowsing() {
	if g.browser != nil {
		g.browser.Close()
	}
	g.browser, g.browseErr = nil, nil
}

// foundGames is the gbe hosting on the local network, as heard lately.
func (g *Game) foundGames() []link.Found {
	if g.browser == nil {
		return nil
	}
	return g.browser.Games()
}

// pollCable plugs the link cable in when a connection is made, and unplugs
// it when the connection ends. It runs even while the menu is open, so the
// other side need not wait.
func (g *Game) pollCable() {
	if g.browser != nil && (!g.menu.open || g.menu.page != pageJoin) {
		g.stopBrowsing() // the menu was closed on the Join page
	}
	if g.cable == nil {
		return
	}
	if g.link != nil {
		select {
		case <-g.link.Done():
			g.link = nil
			if g.gb != nil {
				g.gb.SetLink(nil)
			}
			log.Print("link cable disconnected")
			g.notify(g.tr().T("toast.link_disconnected"))
		default:
		}
		return
	}
	select {
	case l := <-g.cable.Links():
		g.link = l
		g.plugCable()
		log.Print("link cable connected")
		g.notify(g.tr().T("toast.link_connected"))
	default:
	}
}

// plugCable plugs the link cable, if connected, into the console, which may
// be a new one.
func (g *Game) plugCable() {
	if g.link != nil && g.gb != nil {
		g.gb.SetLink(g.link)
	}
}

// linkBlocks tells, while the link cable is connected, that the action
// cannot be used: rewinding, fast forwarding or loading a state would
// break the step the two Game Boys keep. held is whether the action is
// asked for this tick; the notification shows once per press.
func (g *Game) linkBlocks(held bool) bool {
	if g.link == nil || !held {
		g.linkHinted = false
		return false
	}
	if !g.linkHinted {
		g.notify(g.tr().T("toast.link_blocked"))
		g.linkHinted = true
	}
	return true
}

// runFrame runs a frame of the console, through the link cable when it is
// connected.
func (g *Game) runFrame() {
	if g.link != nil {
		g.gb.RunLinkedFrame()
		return
	}
	g.gb.RunFrame()
}
