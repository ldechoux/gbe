package ui

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ldechoux/gbe/internal/link"
	"github.com/ldechoux/gbe/internal/rom"
)

// fakeBrowser lists the games of the test.
type fakeBrowser struct {
	games  []link.Found
	closed bool
}

func (b *fakeBrowser) Games() []link.Found { return b.games }
func (b *fakeBrowser) Close()              { b.closed = true }

// withCables has the menu make fake ends of the cable: hosting fails with
// hostErr, and the browser finds games.
func withCables(g *Game, hostErr error, games ...link.Found) (made *[]*fakeCable, browser *fakeBrowser) {
	made = &[]*fakeCable{}
	browser = &fakeBrowser{games: games}
	g.cables = cableMakers{
		host: func() (CableEnd, error) {
			if hostErr != nil {
				return nil, hostErr
			}
			c := newFakeCable(true)
			*made = append(*made, c)
			return c, nil
		},
		join: func(addr string) CableEnd {
			c := newFakeCable(false)
			c.target = addr
			*made = append(*made, c)
			return c
		},
		browse: func() (gameBrowser, error) {
			browser.closed = false
			return browser, nil
		},
	}
	return made, browser
}

// press runs the menu for one tick with the actions.
func press(g *Game, a menuActions) {
	g.actions = a
	g.pollCable()
	g.menu.update(g)
}

// items is what the page shows: its entries and its footer.
func items(g *Game) ([]string, string) {
	v := g.menu.view(g)
	return v.items, v.footer
}

func TestLinkMenuHost(t *testing.T) {
	g := withGameBoy(t, newTestGame(t, newFakePads()))
	g.title = "TETRIS"
	made, _ := withCables(g, nil)
	l := g.tr()

	// From the main page.
	g.menu.show()
	g.menu.cursor = itemLink
	if got := entryLabel(g, itemLink); got != l.T("menu.link") {
		t.Fatalf("main entry %q", got)
	}
	press(g, menuActions{ok: true})
	got, footer := items(g)
	if g.menu.page != pageLink || !slices.Equal(got, []string{l.T("link.host"), l.T("link.join"), l.T("link.back")}) || footer != l.T("link.help") {
		t.Fatalf("page %d: %q, %q", g.menu.page, got, footer)
	}

	// Host: waiting, the game announced, the address told.
	press(g, menuActions{ok: true})
	if len(*made) != 1 || g.cable != (*made)[0] || (*made)[0].title != "TETRIS" {
		t.Fatalf("hosting: %d cables, title %q", len(*made), (*made)[0].title)
	}
	got, footer = items(g)
	if !slices.Equal(got, []string{l.T("link.cancel"), l.T("link.back")}) || !strings.HasPrefix(footer, l.T("link.waiting")) ||
		!strings.Contains(footer, l.T("link.address", "")) {
		t.Errorf("waiting: %q, %q", got, footer)
	}

	// Connected: the page and the badge tell it.
	other := connect(t, g, (*made)[0])
	got, footer = items(g)
	if !slices.Equal(got, []string{l.T("link.unplug"), l.T("link.back")}) || !strings.HasPrefix(footer, l.T("link.connected", "pc")) {
		t.Errorf("connected: %q, %q", got, footer)
	}
	if g.speedBadge() != "LINK" {
		t.Errorf("badge %q", g.speedBadge())
	}

	// Unplug.
	press(g, menuActions{ok: true})
	if !(*made)[0].closed || g.cable != nil || g.link != nil || g.gb.Link() != nil {
		t.Error("still plugged in after Unplug")
	}
	if got, _ := items(g); got[0] != l.T("link.host") {
		t.Errorf("after Unplug: %q", got)
	}
	other.Close()

	// Back to the main page, on the entry.
	press(g, menuActions{back: true})
	if g.menu.page != pageMain || g.menu.cursor != itemLink {
		t.Errorf("back: page %d, cursor %d", g.menu.page, g.menu.cursor)
	}
}

func TestLinkMenuHostFails(t *testing.T) {
	g := newTestGame(t, newFakePads())
	withCables(g, errors.New("address already in use"))
	g.menu.show()
	g.menu.showLink(g)
	press(g, menuActions{ok: true})
	_, footer := items(g)
	if g.cable != nil || !strings.HasPrefix(footer, g.tr().T("link.host_failed", "address already in use")) {
		t.Errorf("cable %v, footer %q", g.cable, footer)
	}
}

func TestLinkMenuJoin(t *testing.T) {
	g := newTestGame(t, newFakePads()) // no game yet: joining from the drop screen
	mac := link.Found{Name: "mac", Title: "TETRIS", Addr: "192.168.1.20:5555"}
	pc := link.Found{Name: "pc", Addr: "192.168.1.21:5555"}
	made, browser := withCables(g, nil)
	l := g.tr()
	g.menu.show()
	g.menu.showLink(g)
	g.menu.cursor = linkJoin
	press(g, menuActions{ok: true})
	if g.menu.page != pageJoin || g.browser == nil {
		t.Fatalf("page %d, browser %v", g.menu.page, g.browser)
	}
	if got, footer := items(g); !slices.Equal(got, []string{l.T("link.back")}) || !strings.Contains(footer, l.T("link.none")) {
		t.Errorf("nothing found: %q, %q", got, footer)
	}

	// The games come as they are heard.
	browser.games = []link.Found{mac, pc}
	want := []string{"mac - TETRIS", "pc - " + l.T("link.no_game"), l.T("link.back")}
	if got, _ := items(g); !slices.Equal(got, want) {
		t.Errorf("found: %q, want %q", got, want)
	}
	press(g, menuActions{down: true})
	press(g, menuActions{ok: true})
	if len(*made) != 1 || (*made)[0].target != pc.Addr || g.linkTarget != "pc" || !browser.closed {
		t.Fatalf("joined %+v, target %q, browser closed %v", *made, g.linkTarget, browser.closed)
	}
	if _, footer := items(g); g.menu.page != pageLink || !strings.HasPrefix(footer, l.T("link.joining", "pc")) {
		t.Errorf("joining: page %d, %q", g.menu.page, footer)
	}

	// Cancel.
	press(g, menuActions{ok: true})
	if !(*made)[0].closed || g.cable != nil {
		t.Error("still connecting after Cancel")
	}

	// Leaving the list, or the menu, stops looking.
	g.menu.cursor = linkJoin
	press(g, menuActions{ok: true})
	press(g, menuActions{back: true})
	if !browser.closed || g.browser != nil || g.menu.page != pageLink {
		t.Errorf("back from the list: browser %v, page %d", g.browser, g.menu.page)
	}
	g.menu.cursor = linkJoin
	press(g, menuActions{ok: true})
	press(g, menuActions{toggle: true})
	g.pollCable()
	if !browser.closed || g.browser != nil {
		t.Error("still looking once the menu is closed")
	}
}

func TestLinkMenuBrowseFails(t *testing.T) {
	g := newTestGame(t, newFakePads())
	withCables(g, nil)
	g.cables.browse = func() (gameBrowser, error) { return nil, errors.New("address already in use") }
	g.menu.show()
	g.menu.showLink(g)
	g.menu.cursor = linkJoin
	press(g, menuActions{ok: true})
	if _, footer := items(g); footer != g.tr().T("link.browse_failed", "address already in use") {
		t.Errorf("footer %q", footer)
	}
}

// A cable from the command line shows on the page, and a new game is
// announced by its title.
func TestLinkMenuCommandLine(t *testing.T) {
	g := withGameBoy(t, newTestGame(t, newFakePads()))
	c := newFakeCable(false)
	c.target = "192.168.1.20:5555"
	g.cable = c
	g.menu.show()
	g.menu.showLink(g)
	if _, footer := items(g); !strings.HasPrefix(footer, g.tr().T("link.joining", "192.168.1.20:5555")) {
		t.Errorf("footer %q", footer)
	}
	next, err := rom.Open(writeROM(t, filepath.Join(t.TempDir(), "Next.gb"), "NEXT", 0x03), rom.Options{BootROM: "none"})
	if err != nil {
		t.Fatal(err)
	}
	g.startGame(next)
	if c.title != next.Title {
		t.Errorf("announced %q, want %q", c.title, next.Title)
	}
}

func TestLinkMenuDetails(t *testing.T) {
	g := withGameBoy(t, newTestGame(t, newFakePads()))
	made, browser := withCables(g, nil)
	g.menu.show()
	g.menu.showLink(g)
	press(g, menuActions{up: true}) // wraps to Back
	if g.menu.cursor != linkBack {
		t.Errorf("up from the first entry: %d", g.menu.cursor)
	}
	press(g, menuActions{down: true})
	press(g, menuActions{down: true})
	if g.menu.cursor != linkJoin {
		t.Errorf("down twice: %d", g.menu.cursor)
	}

	// Back from the list, through its entry.
	browser.games = []link.Found{{Name: "a very long machine name", Title: "POKEMON YELLOW", Addr: "10.0.0.2:5555"}}
	press(g, menuActions{ok: true})
	if got, _ := items(g); got[0] != "a very long mac. - POKEMON YELLOW" {
		t.Errorf("long name: %q", got[0])
	}
	press(g, menuActions{down: true})
	press(g, menuActions{ok: true})
	if g.menu.page != pageLink || len(*made) != 0 {
		t.Errorf("Back entry: page %d, %d cables", g.menu.page, len(*made))
	}
	if a := hostAddress(1234); !strings.HasSuffix(a, ":1234") {
		t.Errorf("address with another port: %q", a)
	}

	// A frame runs through the cable once connected.
	press(g, menuActions{ok: true}) // Host, where the cursor is back
	connect(t, g, (*made)[0])
	g.runFrame()
	if g.gb.Link() == nil {
		t.Error("the frame ran without the cable")
	}
}
