package ui

import (
	"log"

	"github.com/ldechoux/gbe/internal/gb"
)

// CableEnd is the end of the link cable gbe was launched with (see
// link.Cable): each connection made comes through Links.
type CableEnd interface {
	Links() <-chan *gb.Link
}

// pollCable plugs the link cable in when a connection is made, and unplugs
// it when the connection ends. It runs even while the menu is open, so the
// other side need not wait.
func (g *Game) pollCable() {
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
