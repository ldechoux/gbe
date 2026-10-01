package ui

import (
	"bytes"
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/gomono"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/i18n"
)

// The bitmap font is ASCII only, hence unaccented translations (see
// internal/i18n). Key names of the active layout may contain other
// characters (e.g. "é" on AZERTY): they fall back to Go Mono.

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
	itemLanguage
	itemSaveState
	itemLoadState
	itemReset
	itemQuit
	mainItems
)

// paletteLabel is the palette entry of the main page. In Game Boy Color mode
// it switches the color correction instead.
func paletteLabel(l *i18n.Locale, g *Game) string {
	if !g.colorMode() {
		return l.T("menu.palette", g.palette().Name)
	}
	if g.cfg.ColorCorrection {
		return l.T("menu.colors", l.T("colors.corrected"))
	}
	return l.T("menu.colors", l.T("colors.raw"))
}

// buttonLabel names a Game Boy button in language l.
func buttonLabel(l *i18n.Locale, b gb.Button) string {
	return l.T("button." + strings.ToLower(b.String()))
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

// controlsTab is the device shown on the controls page.
type controlsTab int

const (
	tabKeyboard controlsTab = iota
	tabPad
)

type menu struct {
	open      bool
	page      menuPage
	tab       controlsTab
	cursor    int
	capturing bool   // waiting for a key or button to bind to the selected entry
	notice    string // why the last capture was refused, or a status message
	// width is the widest content drawn since the page was opened: the
	// panel never shrinks while its text changes (tabs, notices, captures).
	width float64
}

// Controls page layout: one entry per button, then these.
func controlsScreenshot() int { return len(gb.Buttons) }
func controlsDefaults() int   { return len(gb.Buttons) + 1 }
func controlsBack() int       { return len(gb.Buttons) + 2 }

// Gamepad tab layout: one entry per button, then these.
func padDefaults() int { return len(gb.Buttons) }
func padBack() int     { return len(gb.Buttons) + 1 }

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
		if m.tab == tabPad {
			return padBack() + 1
		}
		return controlsBack() + 1
	case pageStart:
		return startItems
	}
	return mainItems
}

// backToMain leaves the controls page.
func (m *menu) backToMain() {
	m.page, m.tab, m.cursor, m.notice, m.width = pageMain, tabKeyboard, itemControls, "", 0
}

// update reacts to this tick's actions (see menuActions); captures read the
// raw keys and buttons instead.
func (m *menu) update(g *Game) {
	if m.page == pageControls && m.tab == tabPad && len(g.pads.ids()) == 0 {
		m.tab, m.cursor, m.capturing, m.notice = tabKeyboard, 0, false, g.tr().T("controls.pad_disconnected")
	}
	if m.capturing {
		if m.tab == tabPad {
			m.capturePad(g)
		} else {
			m.capture(g)
		}
		return
	}

	a := g.actions
	switch {
	case a.toggle:
		if m.page != pageStart {
			m.open = false
		}
	case a.back:
		switch m.page {
		case pageStart: // a choice is required
		case pageControls:
			m.backToMain()
		default:
			m.open = false
		}
	case a.up:
		m.cursor = (m.cursor + m.itemCount() - 1) % m.itemCount()
	case a.down:
		m.cursor = (m.cursor + 1) % m.itemCount()
	case a.left:
		m.adjust(g, -1)
	case a.right:
		m.adjust(g, 1)
	case a.ok:
		m.activate(g)
	}
}

// capturePad waits for the gamepad button to bind to the selected entry.
// Only Escape cancels: every gamepad button can be bound.
func (m *menu) capturePad(g *Game) {
	if pressed(ebiten.KeyEscape) {
		m.capturing, m.notice = false, ""
		return
	}
	if b, ok := padJustPressed(g.pads); ok {
		g.cfg.BindPad(gb.Buttons[m.cursor], b)
		g.saveConfig()
		m.capturing, m.notice = false, ""
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
	if found {
		m.captureKey(g, currentHotkey(key))
	}
}

// captureKey assigns combo, pressed while capturing, to the selected entry.
func (m *menu) captureKey(g *Game, combo Hotkey) {
	key := combo.Key
	switch {
	case key == ebiten.KeyEscape && !combo.hasModifiers():
		m.capturing, m.notice = false, ""
	case m.cursor == controlsScreenshot():
		if !combo.hasModifiers() && g.cfg.bound(key) {
			m.notice = g.tr().T("controls.key_used_by_button", keyLabel(g.tr(), key))
			return
		}
		g.cfg.Screenshot = combo
		g.saveConfig()
		m.capturing, m.notice = false, ""
	default:
		if g.cfg.conflicts(key) {
			m.notice = g.tr().T("controls.key_used_by_screenshot", keyLabel(g.tr(), key))
			return
		}
		g.cfg.Bind(gb.Buttons[m.cursor], key)
		g.saveConfig()
		m.capturing, m.notice = false, ""
	}
}

func (m *menu) adjust(g *Game, delta int) {
	if m.page == pageControls {
		m.switchTab(g, delta)
		return
	}
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
	case itemLanguage:
		g.cycleLanguage(delta)
		m.width = 0 // the labels change length
	}
}

// switchTab moves between the Keyboard and Gamepad tabs. The latter is only
// reachable while a gamepad is connected; otherwise it is drawn greyed out,
// which is enough feedback.
func (m *menu) switchTab(g *Game, delta int) {
	switch {
	case delta > 0 && m.tab == tabKeyboard:
		if len(g.pads.ids()) == 0 {
			return
		}
		m.tab, m.cursor, m.notice = tabPad, 0, ""
	case delta < 0 && m.tab == tabPad:
		m.tab, m.cursor, m.notice = tabKeyboard, 0, ""
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
	if m.page == pageControls && m.tab == tabPad {
		switch {
		case m.cursor < len(gb.Buttons):
			m.capturing = true
		case m.cursor == padDefaults():
			g.cfg.Gamepad = defaultPad()
			g.saveConfig()
		default:
			m.backToMain()
		}
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
			m.backToMain()
		}
		return
	}
	switch m.cursor {
	case itemResume:
		m.open = false
	case itemPalette, itemVolume, itemScale, itemLanguage:
		m.adjust(g, 1)
	case itemControls:
		m.page, m.cursor, m.width = pageControls, 0, 0
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

// menuTab is one entry of the controls page tab bar.
type menuTab struct {
	label            string
	active, disabled bool
}

// menuView is what draw renders.
type menuView struct {
	title  string
	tabs   []menuTab
	items  []string
	footer string
}

func (m *menu) view(g *Game) menuView {
	if m.page == pageControls {
		name, family, connected := padName(g.pads)
		l := g.tr()
		v := menuView{title: l.T("controls.title"), tabs: []menuTab{
			{label: l.T("controls.keyboard"), active: m.tab == tabKeyboard},
			{label: l.T("controls.gamepad"), active: m.tab == tabPad, disabled: !connected},
		}}
		if m.tab == tabPad {
			v.items, v.footer = m.padLines(g, name, family)
		} else {
			v.items, v.footer = m.keyboardLines(g)
		}
		return v
	}
	title, items, footer := m.lines(g)
	return menuView{title: title, items: items, footer: footer}
}

func (m *menu) padLines(g *Game, name string, family padFamily) (items []string, footer string) {
	l := g.tr()
	for i, b := range gb.Buttons {
		btn := padLabel(l, family, g.cfg.PadButton(b))
		if m.capturing && i == m.cursor {
			btn = l.T("controls.press_button")
		}
		items = append(items, fmt.Sprintf("%-8s %s", buttonLabel(l, b), btn))
	}
	items = append(items, l.T("controls.defaults"), l.T("controls.back"))
	if len(name) > 32 {
		name = name[:31] + "."
	}
	footer = l.T("controls.pad_name", name)
	switch {
	case m.notice != "":
		footer = m.notice
	case m.capturing:
		footer = l.T("controls.cancel")
	}
	return items, footer
}

func (m *menu) keyboardLines(g *Game) (items []string, footer string) {
	l := g.tr()
	for i, b := range gb.Buttons {
		key := keyLabel(l, g.cfg.Key(b))
		if m.capturing && i == m.cursor {
			key = l.T("controls.press_key")
		}
		items = append(items, fmt.Sprintf("%-8s %s", buttonLabel(l, b), key))
	}
	shot := g.cfg.Screenshot.label(l)
	if m.capturing && m.cursor == controlsScreenshot() {
		shot = l.T("controls.press_combo")
	}
	items = append(items, fmt.Sprintf("%-8s %s", l.T("controls.screenshot"), shot),
		l.T("controls.defaults"), l.T("controls.back"))
	footer = l.T("controls.footer")
	switch {
	case m.notice != "":
		footer = m.notice
	case m.capturing:
		footer = l.T("controls.cancel")
	}
	return items, footer
}

func (m *menu) lines(g *Game) (title string, items []string, footer string) {
	l := g.tr()
	if m.page == pageStart {
		footer = l.T("start.footer")
		if !g.stateTime.IsZero() {
			footer = l.T("start.saved_at", g.stateTime.Format(l.T("start.date_format")))
		}
		return l.T("start.title"), []string{l.T("start.resume"), l.T("start.fresh")}, footer
	}
	items = []string{
		l.T("menu.resume"),
		paletteLabel(l, g),
		l.T("menu.controls"),
		l.T("menu.volume", int(math.Round(g.cfg.Volume*100))),
		l.T("menu.scale", g.cfg.Scale),
		l.T("menu.language", l.Name),
		l.T("menu.save_state"),
		l.T("menu.load_state"),
		l.T("menu.reset"),
		l.T("menu.quit"),
	}
	footer = l.T("menu.footer")
	if len(g.pads.ids()) > 0 {
		footer = l.T("menu.footer_pad")
	}
	return l.T("menu.title"), items, footer
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

	v := m.view(g)
	title, items, footer := v.title, v.items, v.footer
	lineH := 16 * scale
	pad := 10 * scale
	tabGap := 2 * text.Advance(" ", menuFace) // space between tabs, before scaling
	width := text.Advance(footer, menuFace)
	for _, s := range append([]string{title}, items...) {
		width = math.Max(width, text.Advance("  "+s, menuFace))
	}
	tabsW := 0.0
	for _, t := range v.tabs {
		tabsW += text.Advance(" "+t.label+" ", menuFace) + tabGap
	}
	width = math.Max(width, tabsW)
	width = math.Max(width, m.width)
	m.width = width
	w := width*scale + 2*pad
	h := float64(len(items)+3)*lineH + 2*pad
	if len(v.tabs) > 0 {
		h += lineH * 1.5
	}
	x, y := math.Round((sw-w)/2), math.Round((sh-h)/2)

	shadow := pal[3]
	shadow.A = 0xB0
	fillRect(dst, 0, 0, sw, sh, color.NRGBA{shadow.R, shadow.G, shadow.B, shadow.A})
	fillRect(dst, x-scale*2, y-scale*2, w+scale*4, h+scale*4, pal[3])
	fillRect(dst, x, y, w, h, pal[0])

	ty := y + pad
	drawText(dst, title, x+pad, ty, scale, pal[3])
	ty += lineH * 1.5
	if len(v.tabs) > 0 {
		tx := x + pad
		for _, t := range v.tabs {
			label := " " + t.label + " "
			tw := text.Advance(label, menuFace) * scale
			fg := pal[3]
			switch {
			case t.active:
				fillRect(dst, tx, ty-2*scale, tw, lineH, pal[3])
				fg = pal[0]
			case t.disabled:
				fg = pal[1] // faded: no gamepad connected
			}
			drawText(dst, label, tx, ty, scale, fg)
			tx += tw + tabGap*scale
		}
		fillRect(dst, x+pad/2, ty+lineH-scale, w-pad, scale, pal[3]) // underline the tab bar
		ty += lineH * 1.5
	}
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
