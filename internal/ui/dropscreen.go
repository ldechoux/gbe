package ui

import (
	"errors"
	"fmt"
	"image/color"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/ldechoux/gbe/internal/menusound"
	"github.com/ldechoux/gbe/internal/rom"
)

const (
	// maxRecent is how many recent games the drop screen offers.
	maxRecent = 5
	// arrowTicks is how long the arrow of the drop screen stays at each
	// height: it bounces pixel by pixel, like a sprite.
	arrowTicks = 12
)

// pixArt is a small pixel-art picture, in the 4 colors of a palette.
type pixArt struct {
	w, h int
	c    []int8 // palette index of each pixel, -1 for transparent
}

func newPixArt(w, h int) *pixArt {
	p := &pixArt{w: w, h: h, c: make([]int8, w*h)}
	for i := range p.c {
		p.c[i] = -1
	}
	return p
}

func (p *pixArt) set(x, y int, c int8) {
	if x >= 0 && y >= 0 && x < p.w && y < p.h {
		p.c[y*p.w+x] = c
	}
}

func (p *pixArt) rect(x, y, w, h int, c int8) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			p.set(i, j, c)
		}
	}
}

// draw draws p at (x, y), each of its pixels s x s pixels large.
func (p *pixArt) draw(dst *ebiten.Image, x, y, s float64, pal *Palette) {
	for j := range p.h {
		for i := range p.w {
			if c := p.c[j*p.w+i]; c >= 0 {
				fillRect(dst, x+float64(i)*s, y+float64(j)*s, s, s, pal.Colors[c])
			}
		}
	}
}

var (
	cartridgeArt = newCartridgeArt()
	arrowArt     = newArrowArt()
)

// newCartridgeArt draws a Game Boy cartridge: the ridges at the top, the
// label, the cut corner and the contacts.
func newCartridgeArt() *pixArt {
	p := newPixArt(22, 26)
	p.rect(0, 0, 22, 26, 3)
	p.rect(1, 1, 20, 24, 1)
	for i := range 3 { // the cut corner, top right
		for j := range 3 - i {
			p.set(21-j, i, -1)
		}
	}
	p.set(19, 1, 3)
	p.set(20, 2, 3)
	p.rect(20, 3, 1, 22, 2) // shade on the right side
	p.rect(3, 3, 15, 1, 2)  // ridges
	p.rect(3, 5, 15, 1, 2)
	p.rect(3, 8, 16, 13, 3) // label
	p.rect(4, 9, 14, 11, 0)
	p.rect(6, 11, 10, 1, 2)
	p.rect(6, 13, 7, 1, 2)
	p.rect(6, 15, 9, 1, 2)
	p.rect(6, 17, 5, 1, 2)
	for x := 3; x < 19; x += 2 { // contacts
		p.set(x, 23, 2)
	}
	return p
}

// newArrowArt draws an arrow pointing down.
func newArrowArt() *pixArt {
	p := newPixArt(9, 9)
	p.rect(3, 0, 3, 5, 3)
	for i := range 4 {
		p.rect(i, 5+i, 9-2*i, 1, 3)
	}
	return p
}

// arrowBounce is the height of the arrow at tick, in pixels of the picture:
// 0, 1, 2, 1, 0...
func arrowBounce(tick int64) int {
	return []int{0, 1, 2, 1}[tick/arrowTicks%4]
}

// drawDropScreen is shown while no game is loaded: a cartridge under a
// bouncing arrow asks for a ROM to be dropped on the window, and the recent
// games are offered below. The texts grow with the window, as long as they
// fit its width, and the cartridge takes the height left.
func (g *Game) drawDropScreen(dst *ebiten.Image) {
	l, pal := g.tr(), g.menuPalette()
	sw, sh := float64(dst.Bounds().Dx()), float64(dst.Bounds().Dy())
	dst.Fill(pal.Colors[0])
	u := math.Max(1, math.Floor(sh/288)) // 1 at x2, 2 at x4, 4 at x8
	margin := 8 * u
	// fit is the text scale of s: want, or less if s would not fit.
	fit := func(s string, want float64) float64 {
		return math.Max(1, math.Min(want, math.Floor((sw-2*margin)/text.Advance(s, menuFace))))
	}
	center := func(s string, y, sc float64, c color.Color) {
		drawText(dst, s, math.Round((sw-text.Advance(s, menuFace)*sc)/2), math.Round(y), sc, c)
	}

	title, formats := l.T("drop.title"), l.T("drop.formats")
	titleScale, textScale := fit(title, 2*u), fit(formats, u)
	hint := l.T("drop.hint")
	recent := g.cfg.Recent
	if g.dropEntries() > 0 {
		hint = l.T("drop.hint_recent")
	}
	// The recent games, then the entry that opens the list of all of them.
	first := 0 // the first entry shown, when they do not all fit
	key := fmt.Sprint(g.recentCursor)
	if g.recentCursor < len(recent) {
		key += recent[g.recentCursor].Path
	}
	tick := g.marqueeTick("home", key)
	names := make([]string, len(recent)+1)
	listScale := u
	for i, r := range recent {
		names[i] = listName(recentName(r), gameNameWidth, i == g.recentCursor, tick)
		listScale = math.Min(listScale, fit(strings.Repeat("x", min(gameNameWidth, len([]rune(recentName(r))))), u))
	}
	names[len(recent)] = l.T("drop.all_games")
	if n := len(g.library.games); n > 0 {
		names[len(recent)] = l.T("drop.all_games_count", n)
	}
	listScale = math.Min(listScale, fit(names[len(recent)], u))
	lineH := 16 * listScale
	// A hint too wide for one line is split where it has wide spaces.
	hintScale := math.Max(1, math.Floor(u/2))
	hintLines := []string{hint}
	if text.Advance(hint, menuFace)*hintScale > sw-2*margin {
		hintLines = strings.Split(hint, "   ")
	}
	hintH := float64(len(hintLines)) * 16 * hintScale

	// Too wide even at scale 1 (x1 without HiDPI), the title goes on
	// several lines.
	titleLines := wrapText(title, (sw-2*margin)/titleScale)
	titleH := float64(len(titleLines)) * 13 * titleScale
	textH := titleH + 6*u + 13*textScale
	avail := sh - hintH - 3*margin
	{
		// In a tiny window (x1 without HiDPI), only the entries that fit,
		// the selected one among them.
		listTop := textH + 14*u
		if len(recent) > 0 {
			listTop += 13*textScale + 8*u
		}
		n := min(len(names), max(1, int((avail-listTop)/lineH)))
		first = max(0, g.recentCursor-n+1)
		names = names[first : first+n]
		textH = listTop + float64(n)*lineH
	}
	// A pixel of the pictures: the arrow (9), a gap (4), the cartridge (26),
	// left out when there is no room for them.
	s := math.Min(math.Floor(sh/90), math.Floor((avail-textH-10*u)/39))
	artH := 39*s + 10*u
	if s < 1 {
		artH = 0
	}
	y := math.Round(margin + (avail-(artH+textH))/2)

	if s >= 1 {
		arrowArt.draw(dst, math.Round((sw-9*s)/2), y+float64(arrowBounce(ebiten.Tick()))*s, s, pal)
		cartridgeArt.draw(dst, math.Round((sw-22*s)/2), y+13*s, s, pal)
	}
	y += artH
	for i, line := range titleLines {
		center(line, y+float64(i)*13*titleScale, titleScale, pal.Colors[3])
	}
	y += titleH + 6*u
	center(formats, y, textScale, pal.Colors[2])
	y += 13 * textScale

	y += 14 * u
	if len(recent) > 0 {
		center(l.T("drop.recent"), y, textScale, pal.Colors[2])
		y += 13*textScale + 8*u
	}
	// The highlight keeps its width while the selected name scrolls.
	width := 0.0
	for _, name := range names {
		width = math.Max(width, text.Advance(name, menuFace)*listScale)
	}
	width = math.Min(sw-2*margin, width+16*listScale)
	for i, name := range names {
		fg := pal.Colors[3]
		switch {
		case first+i == len(recent) && len(g.library.games) == 0:
			fg = pal.Colors[1] // faded: no games folder, or no game in it
		case first+i == g.recentCursor:
			fillRect(dst, math.Round((sw-width)/2), y-2*listScale, width, lineH, pal.Colors[2])
			fg = pal.Colors[0]
		}
		center(name, y, listScale, fg)
		y += lineH
	}
	y = sh - margin - hintH
	for _, line := range hintLines {
		center(line, y, hintScale, pal.Colors[2])
		y += 16 * hintScale
	}
}

// recentName is how the lists name a recent game: as the games of the
// folder, by its file name (see gameName).
func recentName(r RecentGame) string { return gameName(r.Path) }

// dropEntries is how many entries of the drop screen can be selected: the
// recent games, and the list of all of them when the games folder has any.
func (g *Game) dropEntries() int {
	n := len(g.cfg.Recent)
	if len(g.library.games) > 0 {
		n++
	}
	return n
}

// updateDropScreen moves through the recent games, and launches the
// selected one, or opens the list of all the games.
func (g *Game) updateDropScreen() {
	n := g.dropEntries()
	if n == 0 {
		return
	}
	g.recentCursor = min(g.recentCursor, n-1)
	switch a := g.actions; {
	case a.up:
		g.recentCursor = (g.recentCursor + n - 1) % n
		g.menuSound(menusound.Move)
	case a.down:
		g.recentCursor = (g.recentCursor + 1) % n
		g.menuSound(menusound.Move)
	case a.ok && g.recentCursor == len(g.cfg.Recent):
		g.menuSound(menusound.Enter)
		g.menu.show()
		g.menu.showGames(g, tabAll)
		g.menu.home = true // going back returns here
	case a.ok:
		g.menuSound(menusound.Enter)
		g.playRecent(g.recentCursor)
	}
}

// playRecent starts the recent game i, from the drop screen or the Recent
// games page of the menu: the game in progress, if any, is saved first. A
// game that is gone leaves the list.
func (g *Game) playRecent(i int) {
	r := g.cfg.Recent[i]
	if g.gb != nil && sameFile(r.Path, g.romPath) {
		g.notify(g.tr().T("toast.same_game"))
		return
	}
	next, err := g.openGame(r.Path)
	if errors.Is(err, fs.ErrNotExist) {
		g.cfg.Recent = slices.Delete(g.cfg.Recent, i, i+1)
		g.saveConfig()
		g.notifyLong(g.tr().T("error.recent_missing", recentName(r)))
		return
	}
	if err == nil {
		g.switchGame(next)
	}
}

// addRecent puts the game first in the recent games.
func (g *Game) addRecent(game *rom.Game) {
	path, err := filepath.Abs(game.Path)
	if err != nil || !rom.IsFileName(path) {
		return // the drop screen could not open it
	}
	recent := []RecentGame{{Path: path, Title: game.Title}}
	for _, r := range g.cfg.Recent {
		if r.Path != path && len(recent) < maxRecent {
			recent = append(recent, r)
		}
	}
	g.cfg.Recent = recent
	g.recentCursor = 0
	g.saveConfig()
}

// pruneRecent removes the recent games whose file is gone.
func (g *Game) pruneRecent() {
	kept := slices.DeleteFunc(slices.Clone(g.cfg.Recent), func(r RecentGame) bool {
		_, err := os.Stat(r.Path)
		return errors.Is(err, fs.ErrNotExist)
	})
	if len(kept) != len(g.cfg.Recent) {
		g.cfg.Recent = kept
		g.saveConfig()
	}
}
