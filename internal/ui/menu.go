package ui

import (
	"bytes"
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/gomono"

	"gbe/internal/gb"
)

// The bitmap font is ASCII only, hence the unaccented French. Key names of
// the active layout may contain other characters (e.g. "é" on AZERTY): they
// fall back to Go Mono.

type menuPage int

const (
	pageMain menuPage = iota
	pageControls
	pageStart // shown at launch when a save state exists
)

// Start page entries.
const (
	startResume = iota
	startFresh
	startItems
)

// Main page entries.
const (
	itemResume = iota
	itemPalette
	itemControls
	itemVolume
	itemScale
	itemSaveState
	itemLoadState
	itemReset
	itemQuit
	mainItems
)

var buttonLabels = map[gb.Button]string{
	gb.ButtonUp: "Haut", gb.ButtonDown: "Bas", gb.ButtonLeft: "Gauche", gb.ButtonRight: "Droite",
	gb.ButtonA: "A", gb.ButtonB: "B", gb.ButtonStart: "Start", gb.ButtonSelect: "Select",
}

var (
	menuFace = newMenuFace()
	pixel    *ebiten.Image // 1x1 white, created on first use
)

func newMenuFace() text.Face {
	pixel := text.NewGoXFace(basicfont.Face7x13)
	src, err := text.NewGoTextFaceSource(bytes.NewReader(gomono.TTF))
	if err != nil {
		return pixel
	}
	// 12px Go Mono has about the 7px advance of the bitmap font.
	face, err := text.NewMultiFace(pixel, &text.GoTextFace{Source: src, Size: 12})
	if err != nil {
		return pixel
	}
	return face
}

type menu struct {
	open      bool
	page      menuPage
	cursor    int
	capturing bool   // waiting for a key to bind to the selected entry
	notice    string // why the last captured key was refused
}

// Controls page layout: one entry per button, then these.
func controlsScreenshot() int { return len(gb.Buttons) }
func controlsDefaults() int   { return len(gb.Buttons) + 1 }
func controlsBack() int       { return len(gb.Buttons) + 2 }

func (m *menu) show() {
	*m = menu{open: true}
}

// showStart opens the resume-or-restart prompt.
func (m *menu) showStart() {
	*m = menu{open: true, page: pageStart}
}

func (m *menu) itemCount() int {
	switch m.page {
	case pageControls:
		return controlsBack() + 1
	case pageStart:
		return startItems
	}
	return mainItems
}

func pressed(k ebiten.Key) bool { return inpututil.IsKeyJustPressed(k) }

// repeated is true on press and then periodically while the key is held.
func repeated(k ebiten.Key) bool {
	d := inpututil.KeyPressDuration(k)
	return d == 1 || (d > 20 && d%5 == 0)
}

func (m *menu) update(g *Game) {
	if m.capturing {
		m.capture(g)
		return
	}

	switch {
	case pressed(ebiten.KeyEscape):
		if m.page == pageStart {
			return // a choice is required
		}
		if m.page == pageControls {
			m.page, m.cursor = pageMain, itemControls
		} else {
			m.open = false
		}
		return
	case repeated(ebiten.KeyArrowUp):
		m.cursor = (m.cursor + m.itemCount() - 1) % m.itemCount()
	case repeated(ebiten.KeyArrowDown):
		m.cursor = (m.cursor + 1) % m.itemCount()
	case repeated(ebiten.KeyArrowLeft):
		m.adjust(g, -1)
	case repeated(ebiten.KeyArrowRight):
		m.adjust(g, 1)
	case pressed(ebiten.KeyEnter) || pressed(ebiten.KeyNumpadEnter) || pressed(ebiten.KeySpace):
		m.activate(g)
	}
}

// capture waits for the key to assign to the selected controls entry.
func (m *menu) capture(g *Game) {
	var key ebiten.Key
	found := false
	for _, k := range inpututil.AppendJustPressedKeys(nil) {
		if !isModifier(k) { // modifiers only count as part of a combination
			key, found = k, true
			break
		}
	}
	if !found {
		return
	}
	combo := currentHotkey(key)
	switch {
	case key == ebiten.KeyEscape && !combo.hasModifiers():
		m.capturing, m.notice = false, ""
	case m.cursor == controlsScreenshot():
		if !combo.hasModifiers() && g.cfg.bound(key) {
			m.notice = keyLabel(key) + " sert deja a un bouton"
			return
		}
		g.cfg.Screenshot = combo
		g.saveConfig()
		m.capturing, m.notice = false, ""
	default:
		if g.cfg.conflicts(key) {
			m.notice = keyLabel(key) + " sert deja a la capture"
			return
		}
		g.cfg.Bind(gb.Buttons[m.cursor], key)
		g.saveConfig()
		m.capturing, m.notice = false, ""
	}
}

func (m *menu) adjust(g *Game, delta int) {
	if m.page != pageMain {
		return
	}
	switch m.cursor {
	case itemPalette:
		g.cyclePalette(delta)
	case itemVolume:
		v := math.Round(g.cfg.Volume*10) + float64(delta)
		g.cfg.Volume = max(0, min(10, v)) / 10
		g.player.SetVolume(g.cfg.Volume)
		g.saveConfig()
	case itemScale:
		g.cfg.Scale = max(1, min(maxScale, g.cfg.Scale+delta))
		ebiten.SetWindowSize(gb.ScreenWidth*g.cfg.Scale, gb.ScreenHeight*g.cfg.Scale)
		g.saveConfig()
	}
}

func (m *menu) activate(g *Game) {
	if m.page == pageStart {
		m.open = false
		if m.cursor == startResume {
			g.loadState()
		}
		g.started = true
		return
	}
	if m.page == pageControls {
		switch {
		case m.cursor <= controlsScreenshot():
			m.capturing = true
		case m.cursor == controlsDefaults():
			g.cfg.Keys = defaultKeys()
			g.cfg.Screenshot = defaultScreenshotHotkey()
			g.saveConfig()
		default:
			m.page, m.cursor = pageMain, itemControls
		}
		return
	}
	switch m.cursor {
	case itemResume:
		m.open = false
	case itemPalette, itemVolume, itemScale:
		m.adjust(g, 1)
	case itemControls:
		m.page, m.cursor = pageControls, 0
	case itemSaveState:
		g.saveState()
		m.open = false
	case itemLoadState:
		g.loadState()
		m.open = false
	case itemReset:
		g.saveBattery()
		g.gb.Reset()
		m.open = false
	case itemQuit:
		g.quit = true
		m.open = false
	}
}

func (m *menu) lines(g *Game) (title string, items []string, footer string) {
	if m.page == pageControls {
		for i, b := range gb.Buttons {
			key := keyLabel(g.cfg.Key(b))
			if m.capturing && i == m.cursor {
				key = "appuyez sur une touche..."
			}
			items = append(items, fmt.Sprintf("%-8s %s", buttonLabels[b], key))
		}
		shot := g.cfg.Screenshot.String()
		if m.capturing && m.cursor == controlsScreenshot() {
			shot = "appuyez sur la combinaison..."
		}
		items = append(items, fmt.Sprintf("%-8s %s", "Capture", shot), "Retablir par defaut", "Retour")
		footer = "Entree: changer   Echap: retour"
		switch {
		case m.notice != "":
			footer = m.notice
		case m.capturing:
			footer = "Echap: annuler"
		}
		return "CONTROLES", items, footer
	}
	if m.page == pageStart {
		footer = "Entree: valider"
		if !g.stateTime.IsZero() {
			footer = "Sauvegarde du " + g.stateTime.Format("02/01/2006 a 15:04")
		}
		return "PARTIE EN COURS", []string{"Reprendre la partie", "Recommencer depuis le debut"}, footer
	}
	items = []string{
		"Reprendre",
		fmt.Sprintf("Palette   < %s >", g.palette().Name),
		"Controles...",
		fmt.Sprintf("Volume    < %d%% >", int(math.Round(g.cfg.Volume*100))),
		fmt.Sprintf("Echelle   < x%d >", g.cfg.Scale),
		"Sauvegarder l'etat",
		"Charger l'etat",
		"Reinitialiser",
		"Quitter",
	}
	return "PAUSE", items, "P: palette   F11: plein ecran"
}

// drawToast shows msg in a small box at the bottom-left of the screen.
func drawToast(dst *ebiten.Image, msg string, p *Palette) {
	sh := float64(dst.Bounds().Dy())
	scale := math.Max(1, math.Floor(sh/300))
	pad := 4 * scale
	w := text.Advance(msg, menuFace)*scale + 2*pad
	h := 13*scale + 2*pad
	x, y := pad, sh-h-pad
	fillRect(dst, x, y, w, h, p.Colors[3])
	drawText(dst, msg, x+pad, y+pad, scale, p.Colors[0])
}

func fillRect(dst *ebiten.Image, x, y, w, h float64, c color.Color) {
	if pixel == nil {
		pixel = ebiten.NewImage(1, 1)
		pixel.Fill(color.White)
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(w, h)
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(c)
	dst.DrawImage(pixel, op)
}

func drawText(dst *ebiten.Image, s string, x, y, scale float64, c color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(c)
	text.Draw(dst, s, menuFace, op)
}

// draw renders the menu with the colors of the current palette.
func (m *menu) draw(dst *ebiten.Image, g *Game) {
	pal := g.palette().Colors
	sw, sh := float64(dst.Bounds().Dx()), float64(dst.Bounds().Dy())
	scale := math.Max(1, math.Floor(sh/300))

	title, items, footer := m.lines(g)
	lineH := 16 * scale
	pad := 10 * scale
	width := text.Advance(footer, menuFace)
	for _, s := range append([]string{title}, items...) {
		width = math.Max(width, text.Advance("  "+s, menuFace))
	}
	w := width*scale + 2*pad
	h := float64(len(items)+3)*lineH + 2*pad
	x, y := math.Round((sw-w)/2), math.Round((sh-h)/2)

	shadow := pal[3]
	shadow.A = 0xB0
	fillRect(dst, 0, 0, sw, sh, color.NRGBA{shadow.R, shadow.G, shadow.B, shadow.A})
	fillRect(dst, x-scale*2, y-scale*2, w+scale*4, h+scale*4, pal[3])
	fillRect(dst, x, y, w, h, pal[0])

	ty := y + pad
	drawText(dst, title, x+pad, ty, scale, pal[3])
	ty += lineH * 1.5
	for i, s := range items {
		fg := pal[3]
		if i == m.cursor {
			fillRect(dst, x+pad/2, ty-2*scale, w-pad, lineH, pal[2])
			fg = pal[0]
			s = "> " + s
		} else {
			s = "  " + s
		}
		drawText(dst, s, x+pad, ty, scale, fg)
		ty += lineH
	}
	drawText(dst, footer, x+pad, ty+lineH*0.5, scale, pal[2])
}
