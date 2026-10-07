package ui

import (
	"bytes"
	"fmt"
	"image/color"
	"math"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/gomono"

	"github.com/ldechoux/gbe/internal/audiofx"
	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/i18n"
	"github.com/ldechoux/gbe/internal/menusound"
)

// The bitmap font is ASCII only, hence unaccented translations (see
// internal/i18n). Key names of the active layout may contain other
// characters (e.g. "é" on AZERTY): they fall back to Go Mono.

type menuPage int

const (
	pageMain menuPage = iota
	pageControls
	pageDisplay
	pageSound
	pageStart  // shown at launch when a save state exists
	pageSwitch // shown when a game is dropped on the window during another
	pageRecent // the last games played, to launch one
	pageBootROM
)

// Start page entries.
const (
	startResume = iota
	startFresh
	startItems
)

// Switch page entries.
const (
	switchLaunch = iota
	switchKeep
	switchItems
)

// Main page entries.
const (
	itemResume = iota
	itemRecent
	itemDisplay
	itemControls
	itemSound
	itemBootROM
	itemSpeed
	itemLanguage
	itemSaveState
	itemLoadState
	itemReset
	itemQuit
	mainItems
)

// Display page entries.
const (
	displayPalette = iota
	displayColorize
	displayScale
	displayMonitor
	displayFullscreen
	displayFilter
	displayGhosting
	displayBack
	displayItems
)

// Sound page entries.
const (
	soundVolume = iota
	soundStereo
	soundFilter
	soundMenuSounds
	soundBack
	soundItems
)

// Boot ROM page entries.
const (
	bootChoose = iota
	bootAuto
	bootGB  // information, greyed out
	bootGBC // same
	bootBack
	bootItems
)

// bootDisabled reports whether the Boot ROM page entry i is greyed out: the
// lines that only inform, and the automatic search while it is on.
func bootDisabled(g *Game, i int) bool {
	return i == bootGB || i == bootGBC || i == bootAuto && g.cfg.BootROMDir == ""
}

// soundDisabled reports whether the sound page entry i is greyed out: the
// stereo mode, while the speaker filter makes the sound mono.
func soundDisabled(g *Game, i int) bool {
	return i == soundStereo && g.cfg.AudioFilter == audiofx.Speaker
}

// paletteLabel is the palette entry of the display page. In Game Boy Color mode
// it switches the color correction instead, except for colorized DMG games,
// which have the palettes of the CGB boot ROM.
func paletteLabel(l *i18n.Locale, g *Game) string {
	if g.compat() {
		return l.T("menu.palette", compatPaletteLabel(l, g.compatChoice()))
	}
	if !g.colorMode() {
		return l.T("menu.palette", g.palette().Name)
	}
	if g.cfg.ColorCorrection {
		return l.T("menu.colors", l.T("colors.corrected"))
	}
	return l.T("menu.colors", l.T("colors.raw"))
}

// onOff names a setting's state in language l.
func onOff(l *i18n.Locale, on bool) string {
	if on {
		return l.T("setting.on")
	}
	return l.T("setting.off")
}

// fullscreenLabel names the display mode: full screen or window, or the
// one the window switches to.
func fullscreenLabel(l *i18n.Locale, g *Game) string {
	if g.fullscreen() {
		return l.T("fullscreen.on")
	}
	return l.T("fullscreen.off")
}

// ghostingLabel names a ghosting mode in language l.
func ghostingLabel(l *i18n.Locale, mode string) string {
	if mode == ghostingOff {
		return l.T("setting.off")
	}
	return l.T("ghosting." + mode)
}

// bindingLabel names a Game Boy button or an action (see bindingNames) in
// language l.
func bindingLabel(l *i18n.Locale, name string) string {
	switch name {
	case actionFastForward:
		return l.T("action.fast_forward")
	case actionRewind:
		return l.T("action.rewind")
	}
	return l.T("button." + strings.ToLower(name))
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

// Controls page layout: one entry per button and action, then these.
func controlsScreenshot() int { return len(bindingNames()) }
func controlsDefaults() int   { return len(bindingNames()) + 1 }
func controlsBack() int       { return len(bindingNames()) + 2 }

// Gamepad tab layout: one entry per button and action, then these.
func padVibration() int { return len(bindingNames()) }
func padTest() int      { return len(bindingNames()) + 1 }
func padDefaults() int  { return len(bindingNames()) + 2 }
func padBack() int      { return len(bindingNames()) + 3 }

// padVibrates reports whether the gamepad shown on the Gamepad tab, the
// first one, may vibrate: otherwise its vibration entries are greyed out.
func padVibrates(g *Game) bool {
	ids := g.pads.ids()
	return len(ids) > 0 && g.pads.canVibrate(ids[0])
}

func (m *menu) show() {
	*m = menu{open: true}
}

// showStart opens the resume-or-restart prompt.
func (m *menu) showStart() {
	*m = menu{open: true, page: pageStart}
}

// showSwitch asks whether to launch the game dropped on the window
// (Game.pending) or to keep playing.
func (m *menu) showSwitch() {
	*m = menu{open: true, page: pageSwitch}
}

// keepPlaying forgets the dropped game. The game in progress goes on, or, if
// the player had not chosen yet, the resume prompt comes back.
func (m *menu) keepPlaying(g *Game) {
	g.pending = nil
	if !g.started {
		m.showStart()
		return
	}
	m.open = false
}

// mainDisabled reports whether the main page entry i is greyed out: those
// about the game, while there is none.
func mainDisabled(g *Game, i int) bool {
	if i == itemRecent {
		return len(g.cfg.Recent) == 0
	}
	return g.gb == nil && (i == itemSaveState || i == itemLoadState || i == itemReset)
}

// displayEntries lists the entries of the display page shown for this game:
// the colorization setting only applies to DMG games, and the monitor one
// needs a second monitor, e.g. a TV.
func displayEntries(g *Game) []int {
	entries := make([]int, 0, displayItems)
	for i := range displayItems {
		switch {
		case i == displayColorize && !g.colorizable():
		case i == displayMonitor && !g.multiMonitor():
		default:
			entries = append(entries, i)
		}
	}
	return entries
}

// move goes to the previous (-1) or next (1) entry. On the display page,
// the cursor is the entry itself (displayPalette...), whichever are hidden.
func (m *menu) move(g *Game, delta int) {
	if m.page != pageDisplay {
		n := m.itemCount(g)
		m.cursor = (m.cursor + n + delta) % n
		for m.page == pageMain && mainDisabled(g, m.cursor) || m.page == pageSound && soundDisabled(g, m.cursor) ||
			m.page == pageBootROM && bootDisabled(g, m.cursor) {
			m.cursor = (m.cursor + n + delta) % n
		}
		return
	}
	entries := displayEntries(g)
	i := max(0, slices.Index(entries, m.cursor))
	m.cursor = entries[(i+len(entries)+delta)%len(entries)]
}

func (m *menu) itemCount(g *Game) int {
	switch m.page {
	case pageControls:
		if m.tab == tabPad {
			return padBack() + 1
		}
		return controlsBack() + 1
	case pageDisplay:
		return displayItems
	case pageSound:
		return soundItems
	case pageBootROM:
		return bootItems
	case pageStart:
		return startItems
	case pageSwitch:
		return switchItems
	case pageRecent:
		return len(g.cfg.Recent) + 1 // and Back
	}
	return mainItems
}

// backToMain leaves a page opened from the main one, back on its entry.
func (m *menu) backToMain() {
	cursor := itemControls
	switch m.page {
	case pageDisplay:
		cursor = itemDisplay
	case pageSound:
		cursor = itemSound
	case pageBootROM:
		cursor = itemBootROM
	case pageRecent:
		cursor = itemRecent
	}
	m.page, m.tab, m.cursor, m.notice, m.width = pageMain, tabKeyboard, cursor, "", 0
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
		switch m.page {
		case pageStart: // a choice is required
		case pageSwitch:
			g.menuSound(menusound.Back)
			m.keepPlaying(g)
		default:
			g.menuSound(menusound.Back)
			m.open = false
		}
	case a.back:
		switch m.page {
		case pageStart: // a choice is required
		case pageSwitch:
			g.menuSound(menusound.Back)
			m.keepPlaying(g)
		case pageControls, pageDisplay, pageSound, pageRecent, pageBootROM:
			g.menuSound(menusound.Back)
			m.backToMain()
		default:
			g.menuSound(menusound.Back)
			m.open = false
		}
	case a.up:
		m.moveSounding(g, -1)
	case a.down:
		m.moveSounding(g, 1)
	case a.left:
		m.adjustSounding(g, -1)
	case a.right:
		m.adjustSounding(g, 1)
	case a.ok:
		m.activate(g)
	}
}

// capturePad waits for the gamepad button to bind to the selected entry.
// Only Escape cancels: every gamepad button can be bound.
func (m *menu) capturePad(g *Game) {
	if pressed(ebiten.KeyEscape) {
		g.menuSound(menusound.Back)
		m.capturing, m.notice = false, ""
		return
	}
	if b, ok := padJustPressed(g.pads); ok {
		g.cfg.BindPad(bindingNames()[m.cursor], b)
		g.saveConfig()
		g.menuSound(menusound.Change)
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
		g.menuSound(menusound.Back)
		m.capturing, m.notice = false, ""
	case m.cursor == controlsScreenshot():
		if !combo.hasModifiers() && g.cfg.bound(key) {
			g.menuSound(menusound.Refuse)
			m.notice = g.tr().T("controls.key_used_by_button", keyLabel(g.tr(), key))
			return
		}
		g.cfg.Screenshot = combo
		g.saveConfig()
		g.menuSound(menusound.Change)
		m.capturing, m.notice = false, ""
	default:
		if g.cfg.conflicts(key) {
			g.menuSound(menusound.Refuse)
			m.notice = g.tr().T("controls.key_used_by_screenshot", keyLabel(g.tr(), key))
			return
		}
		g.cfg.Bind(bindingNames()[m.cursor], key)
		g.saveConfig()
		g.menuSound(menusound.Change)
		m.capturing, m.notice = false, ""
	}
}

func (m *menu) adjust(g *Game, delta int) {
	if m.page == pageControls {
		if m.tab == tabPad && m.cursor == padVibration() {
			m.toggleVibration(g)
			return
		}
		m.switchTab(g, delta)
		return
	}
	if m.page == pageDisplay {
		m.adjustDisplay(g, delta)
		return
	}
	if m.page == pageSound {
		m.adjustSound(g, delta)
		return
	}
	if m.page != pageMain {
		return
	}
	switch m.cursor {
	case itemSpeed:
		g.cfg.FastForwardSpeed = max(minFastForward, min(maxFastForward, g.cfg.FastForwardSpeed+delta))
		g.saveConfig()
	case itemLanguage:
		g.cycleLanguage(delta)
		m.width = 0 // the labels change length
	}
}

// moveSounding moves the cursor, with the Move sound when it goes somewhere.
func (m *menu) moveSounding(g *Game, delta int) {
	cursor := m.cursor
	m.move(g, delta)
	if m.cursor != cursor {
		g.menuSound(menusound.Move)
	}
}

// adjustSounding changes the selected setting by delta, with the Change
// sound when its value changes, Move when the controls page switches tab,
// and Refuse for a setting at its end ("< x8 >" cannot go further) or
// locked; entries that are not settings stay silent.
func (m *menu) adjustSounding(g *Game, delta int) {
	before, tab := m.selectedText(g), m.tab
	m.adjust(g, delta)
	switch after := m.selectedText(g); {
	case m.tab != tab:
		g.menuSound(menusound.Move)
	case after != before:
		g.menuSound(menusound.Change)
	case strings.Contains(before, "<"), m.page == pageControls && m.tab == tabPad && m.cursor == padVibration():
		g.menuSound(menusound.Refuse)
	}
}

// selectedText is the text of the selected entry, as drawn.
func (m *menu) selectedText(g *Game) string {
	v := m.view(g)
	if v.selected < 0 || v.selected >= len(v.items) {
		return ""
	}
	return v.items[v.selected]
}

func (m *menu) adjustDisplay(g *Game, delta int) {
	switch m.cursor {
	case displayPalette:
		g.cyclePalette(delta)
	case displayColorize:
		g.cfg.ColorizeDMG = !g.cfg.ColorizeDMG // applied by Reset (see the footer)
		g.saveConfig()
	case displayScale:
		g.cfg.Scale = max(1, min(maxScale, g.cfg.Scale+delta))
		g.applyScale()
		g.saveConfig()
	case displayMonitor:
		g.cycleMonitor(delta)
	case displayFullscreen:
		g.toggleFullscreen() // like F11, from a gamepad too
	case displayFilter:
		g.cycleFilter(delta)
	case displayGhosting:
		n := len(ghostingModes)
		i := slices.Index(ghostingModes, g.cfg.Ghosting)
		g.cfg.Ghosting = ghostingModes[(i+delta%n+n)%n]
		g.saveConfig()
	}
}

func (m *menu) adjustSound(g *Game, delta int) {
	switch {
	case m.cursor == soundVolume:
		v := math.Round(g.cfg.Volume*10) + float64(delta)
		g.cfg.Volume = max(0, min(10, v)) / 10
		if g.player != nil { // none in the tests
			g.player.SetVolume(g.cfg.Volume)
		}
	case m.cursor == soundStereo && !soundDisabled(g, soundStereo):
		g.cfg.Stereo = cycle(audiofx.StereoModes, g.cfg.Stereo, delta)
	case m.cursor == soundFilter:
		g.cfg.AudioFilter = cycle(audiofx.Filters, g.cfg.AudioFilter, delta)
	case m.cursor == soundMenuSounds:
		g.cfg.MenuSounds = !g.cfg.MenuSounds
	default:
		return
	}
	if g.fx != nil {
		g.fx.Set(g.cfg.Stereo, g.cfg.AudioFilter)
	}
	g.saveConfig()
}

// cycle returns the ID delta places after id in ids, wrapping around.
func cycle(ids []string, id string, delta int) string {
	n := len(ids)
	i := max(0, slices.Index(ids, id))
	return ids[(i+delta%n+n)%n]
}

// toggleVibration turns the gamepad vibrations on or off.
func (m *menu) toggleVibration(g *Game) {
	if padVibrates(g) {
		g.cfg.Vibration = !g.cfg.Vibration
		g.saveConfig()
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
	if m.page == pageSwitch {
		if m.cursor == switchLaunch && g.pending != nil {
			g.menuSound(menusound.Enter)
			g.switchGame(g.pending) // opens the resume prompt, or closes the menu
		} else {
			g.menuSound(menusound.Back)
			m.keepPlaying(g)
		}
		return
	}
	if m.page == pageMain && mainDisabled(g, m.cursor) {
		g.menuSound(menusound.Refuse)
		return
	}
	if m.page == pageRecent {
		if m.cursor >= len(g.cfg.Recent) {
			g.menuSound(menusound.Back)
			m.backToMain()
		} else {
			g.menuSound(menusound.Enter)
			g.playRecent(m.cursor) // closes the menu once launched
		}
		return
	}
	if m.page == pageStart {
		g.menuSound(menusound.Enter)
		m.open = false
		switch {
		case m.cursor == startResume:
			g.loadState()
		case g.modePending():
			g.restart() // starting over: in the mode of the settings
		}
		g.started = true
		return
	}
	if m.page == pageControls && m.tab == tabPad {
		switch {
		case m.cursor < padVibration():
			g.menuSound(menusound.Enter)
			m.capturing = true
		case m.cursor == padVibration():
			m.adjustSounding(g, 1) // toggles the vibrations
		case m.cursor == padTest():
			if padVibrates(g) {
				g.menuSound(menusound.Enter)
				testVibration(g.pads)
			} else {
				g.menuSound(menusound.Refuse)
			}
		case m.cursor == padDefaults():
			g.menuSound(menusound.Enter)
			g.cfg.Gamepad = defaultPad()
			g.cfg.Vibration = true
			g.saveConfig()
		default:
			g.menuSound(menusound.Back)
			m.backToMain()
		}
		return
	}
	if m.page == pageDisplay {
		if m.cursor == displayBack {
			g.menuSound(menusound.Back)
			m.backToMain()
		} else {
			m.adjustSounding(g, 1) // OK cycles like Right
		}
		return
	}
	if m.page == pageSound {
		if m.cursor == soundBack {
			g.menuSound(menusound.Back)
			m.backToMain()
		} else if m.cursor != soundVolume { // OK must not turn the volume up
			m.adjustSounding(g, 1)
		}
		return
	}
	if m.page == pageBootROM {
		switch m.cursor {
		case bootChoose:
			g.menuSound(menusound.Enter)
			g.chooseBootROMDir()
		case bootAuto:
			if bootDisabled(g, bootAuto) {
				g.menuSound(menusound.Refuse)
				return
			}
			g.menuSound(menusound.Change)
			g.setBootROMDir("")
			m.cursor = bootChoose // the entry is greyed out now
		default:
			g.menuSound(menusound.Back)
			m.backToMain()
		}
		return
	}
	if m.page == pageControls {
		switch {
		case m.cursor <= controlsScreenshot():
			g.menuSound(menusound.Enter)
			m.capturing = true
		case m.cursor == controlsDefaults():
			g.menuSound(menusound.Enter)
			g.cfg.Keys = defaultKeys()
			g.cfg.Screenshot = defaultScreenshotHotkey()
			g.saveConfig()
		default:
			g.menuSound(menusound.Back)
			m.backToMain()
		}
		return
	}
	switch m.cursor {
	case itemResume:
		g.menuSound(menusound.Back)
		m.open = false
		return
	case itemSpeed, itemLanguage:
		m.adjustSounding(g, 1)
		return
	}
	g.menuSound(menusound.Enter) // a page opens, or an action is chosen
	switch m.cursor {
	case itemSound:
		m.page, m.cursor, m.width = pageSound, soundVolume, 0
	case itemBootROM:
		m.page, m.cursor, m.width = pageBootROM, bootChoose, 0
	case itemRecent:
		m.page, m.cursor, m.width = pageRecent, 0, 0
	case itemDisplay:
		m.page, m.cursor, m.width = pageDisplay, displayPalette, 0
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
		g.restart()
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
	title    string
	tabs     []menuTab
	items    []string
	disabled map[int]bool // items drawn greyed out
	selected int          // highlighted item
	footer   string       // one line or more
	// altItems and altFooters are the other entries and footers the page
	// may show as its settings and selection change: the panel keeps the
	// size of the largest, instead of changing as the player goes through
	// them.
	altItems, altFooters []string
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
			v.items, v.disabled, v.footer = m.padLines(g, name, family)
		} else {
			v.items, v.footer = m.keyboardLines(g)
		}
		v.selected = m.cursor
		return v
	}
	title, items, footer := m.lines(g)
	v := menuView{title: title, items: items, selected: m.cursor, footer: footer}
	switch m.page {
	case pageDisplay:
		v.selected = slices.Index(displayEntries(g), m.cursor)
	case pageSound:
		if soundDisabled(g, soundStereo) {
			v.disabled = map[int]bool{soundStereo: true}
		}
		v.altItems, v.altFooters = soundAlternatives(g)
	case pageBootROM:
		v.disabled = map[int]bool{}
		for i := range bootItems {
			if bootDisabled(g, i) {
				v.disabled[i] = true
			}
		}
		v.altItems, v.altFooters = bootAlternatives(g)
	case pageMain:
		for i := range items {
			if mainDisabled(g, i) {
				if v.disabled == nil {
					v.disabled = map[int]bool{}
				}
				v.disabled[i] = true
			}
		}
	}
	return v
}

func (m *menu) padLines(g *Game, name string, family padFamily) (items []string, disabled map[int]bool, footer string) {
	l := g.tr()
	for i, name := range bindingNames() {
		btn := padLabel(l, family, g.cfg.Gamepad[name])
		if m.capturing && i == m.cursor {
			btn = l.T("controls.press_button")
		}
		items = append(items, fmt.Sprintf("%-8s %s", bindingLabel(l, name), btn))
	}
	vibrates := padVibrates(g)
	state := onOff(l, g.cfg.Vibration)
	if !vibrates {
		state = l.T("controls.unavailable")
		disabled = map[int]bool{padVibration(): true, padTest(): true}
	}
	items = append(items, l.T("controls.vibration")+"  "+state, l.T("controls.vibration_test"),
		l.T("controls.defaults"), l.T("controls.back"))
	if len(name) > 32 {
		name = name[:31] + "."
	}
	footer = l.T("controls.pad_name", name)
	switch {
	case m.notice != "":
		footer = m.notice
	case m.capturing:
		footer = l.T("controls.cancel")
	case !vibrates && disabled[m.cursor]:
		footer = l.T("controls.no_vibration")
	}
	return items, disabled, footer
}

func (m *menu) keyboardLines(g *Game) (items []string, footer string) {
	l := g.tr()
	for i, name := range bindingNames() {
		key := keyLabel(l, g.cfg.Keys[name])
		if m.capturing && i == m.cursor {
			key = l.T("controls.press_key")
		}
		items = append(items, fmt.Sprintf("%-8s %s", bindingLabel(l, name), key))
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
		if g.modePending() { // the state was made in the other mode
			if g.gb.IsCGB() {
				footer += "\n" + l.T("start.other_mode_color")
			} else {
				footer += "\n" + l.T("start.other_mode_dmg")
			}
		}
		return l.T("start.title"), []string{l.T("start.resume"), l.T("start.fresh")}, footer
	}
	if m.page == pageSwitch {
		name := ""
		if g.pending != nil {
			name = g.pending.Title
			if name == "" {
				name = filepath.Base(g.pending.Path)
			}
		}
		if g.started {
			footer = l.T("switch.footer")
		}
		return l.T("switch.title"), []string{l.T("switch.launch", name), l.T("switch.keep")}, footer
	}
	if m.page == pageRecent {
		for _, r := range g.cfg.Recent {
			name := recentName(r)
			if sameFile(r.Path, g.romPath) {
				name = l.T("recent.current", name)
			}
			items = append(items, name)
		}
		items = append(items, l.T("recent.back"))
		if g.gb != nil && g.started {
			footer = l.T("recent.footer")
		}
		return l.T("recent.title"), items, footer
	}
	if m.page == pageDisplay {
		labels := []string{
			paletteLabel(l, g),
			l.T("menu.colorize", onOff(l, g.cfg.ColorizeDMG)),
			l.T("menu.scale", g.cfg.Scale),
			monitorEntry(l, g),
			l.T("menu.fullscreen", fullscreenLabel(l, g)),
			l.T("menu.filter", l.T("filter."+g.filter().ID)),
			l.T("menu.ghosting", ghostingLabel(l, g.cfg.Ghosting)),
			l.T("display.back"),
		}
		for _, e := range displayEntries(g) {
			items = append(items, labels[e])
		}
		return l.T("display.title"), items, mainFooter(g)
	}
	if m.page == pageSound {
		stereo := g.cfg.Stereo
		if soundDisabled(g, soundStereo) {
			stereo = audiofx.Mono
		}
		items = []string{
			l.T("menu.volume", int(math.Round(g.cfg.Volume*100))),
			l.T("menu.stereo", l.T("stereo."+stereo)),
			l.T("menu.audio_filter", l.T("audio_filter."+g.cfg.AudioFilter)),
			l.T("menu.menu_sounds", onOff(l, g.cfg.MenuSounds)),
			l.T("sound.back"),
		}
		return l.T("sound.title"), items, m.soundFooter(g)
	}
	if m.page == pageBootROM {
		search := g.bootROMSearch()
		found := func(model gb.Model) string {
			if search.Find(model) != "" {
				return l.T("boot_rom.found")
			}
			return l.T("boot_rom.missing")
		}
		items = []string{
			l.T("boot_rom.choose"),
			l.T("boot_rom.auto"),
			l.T("boot_rom.gb", found(gb.ModelDMG)),
			l.T("boot_rom.gbc", found(gb.ModelCGB)),
			l.T("boot_rom.back"),
		}
		return l.T("boot_rom.title"), items, bootFooter(g, g.cfg.BootROMDir, g.bootROMPending)
	}
	items = []string{
		l.T("menu.resume"),
		l.T("menu.recent"),
		l.T("menu.display"),
		l.T("menu.controls"),
		l.T("menu.sound"),
		l.T("menu.boot_rom"),
		l.T("menu.fast_forward", g.cfg.FastForwardSpeed),
		l.T("menu.language", l.Name),
		l.T("menu.save_state"),
		l.T("menu.load_state"),
		l.T("menu.reset"),
		l.T("menu.quit"),
	}
	return l.T("menu.title"), items, mainFooter(g)
}

// soundAlternatives lists every value the entries of the sound page and
// every explanation its footer may show.
func soundAlternatives(g *Game) (items, footers []string) {
	l := g.tr()
	items = append(items, l.T("menu.volume", 100))
	for _, id := range audiofx.StereoModes {
		items = append(items, l.T("menu.stereo", l.T("stereo."+id)))
		footers = append(footers, l.T("sound.help_"+id))
	}
	for _, id := range audiofx.Filters {
		items = append(items, l.T("menu.audio_filter", l.T("audio_filter."+id)))
		footers = append(footers, l.T("sound.help_"+id))
	}
	for _, on := range []bool{true, false} {
		items = append(items, l.T("menu.menu_sounds", onOff(l, on)))
	}
	footers = append(footers, l.T("sound.help_menu_sounds"))
	return items, append(footers, mainFooter(g))
}

// bootDirWidth is the longest folder the Boot ROM page shows, in characters:
// a longer one scrolls.
const bootDirWidth = 30

// bootFooter is the footer of the Boot ROM page: the folder chosen, and the
// reminder to reset when the running game does not use it yet.
func bootFooter(g *Game, dir string, pending bool) string {
	l := g.tr()
	footer := l.T("boot_rom.folder_auto")
	if dir != "" {
		footer = l.T("boot_rom.folder", marquee(shortDir(dir), bootDirWidth, int64(ebiten.Tick())))
	}
	if pending {
		footer += "\n" + l.T("menu.footer_pending")
	}
	return footer
}

// bootAlternatives lists the other values of the Boot ROM page: the panel
// keeps its size when a boot ROM is found or a folder is chosen.
func bootAlternatives(g *Game) (items, footers []string) {
	l := g.tr()
	for _, s := range []string{l.T("boot_rom.found"), l.T("boot_rom.missing")} {
		items = append(items, l.T("boot_rom.gb", s), l.T("boot_rom.gbc", s))
	}
	long := strings.Repeat("x", bootDirWidth)
	for _, pending := range []bool{false, true} {
		footers = append(footers, bootFooter(g, "", pending), bootFooter(g, long, pending))
	}
	return items, footers
}

// soundFooter explains the stereo mode or the filter selected on the sound
// page.
func (m *menu) soundFooter(g *Game) string {
	l := g.tr()
	switch m.cursor {
	case soundStereo:
		return l.T("sound.help_" + g.cfg.Stereo)
	case soundFilter:
		return l.T("sound.help_" + g.cfg.AudioFilter)
	case soundMenuSounds:
		return l.T("sound.help_menu_sounds")
	}
	return mainFooter(g)
}

// mainFooter is the footer of the main and display pages: the keyboard
// shortcuts, or the reminder to reset while a setting waits for it.
func mainFooter(g *Game) string {
	l := g.tr()
	key := "menu.footer"
	if g.colorMode() && !g.compat() {
		key = "menu.footer_color" // P toggles the color correction
	}
	if len(g.pads.ids()) > 0 {
		key += "_pad"
	}
	if g.modePending() || g.bootROMPending {
		return l.T("menu.footer_pending") // e.g. after changing the colorization
	}
	return l.T(key)
}

// drawToast shows msg in a small box at the bottom-left of the screen.
func drawToast(dst *ebiten.Image, msg string, p *Palette) { drawBox(dst, msg, p, false) }

// drawBadge shows msg in a small box at the top-right of the screen.
func drawBadge(dst *ebiten.Image, msg string, p *Palette) { drawBox(dst, msg, p, true) }

func drawBox(dst *ebiten.Image, msg string, p *Palette, topRight bool) {
	sw, sh := float64(dst.Bounds().Dx()), float64(dst.Bounds().Dy())
	scale := math.Max(1, math.Floor(sh/300))
	pad := 4 * scale
	// Too wide for the screen, the message goes on several lines.
	lines := wrapText(msg, (sw-4*pad)/scale)
	width := 0.0
	for _, line := range lines {
		width = math.Max(width, advance(line))
	}
	w := width*scale + 2*pad
	h := float64(len(lines))*13*scale + 2*pad
	x, y := pad, sh-h-pad
	if topRight {
		x, y = sw-w-pad, pad
	}
	fillRect(dst, x, y, w, h, p.Colors[3])
	for i, line := range lines {
		drawText(dst, line, x+pad, y+pad+float64(i)*13*scale, scale, p.Colors[0])
	}
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

// draw renders the menu with the colors of menuPalette.
func (m *menu) draw(dst *ebiten.Image, g *Game) {
	pal := g.menuPalette().Colors
	sw, sh := float64(dst.Bounds().Dx()), float64(dst.Bounds().Dy())

	v := m.view(g)
	l := layoutMenu(v, m.width, sw, sh)
	m.width = l.width
	scale, lineH, pad := l.scale, l.lineH, l.pad
	x, y, w, h := l.x, l.y, l.w, l.h
	tabGap := 2 * advance(" ") // space between tabs, before scaling

	shadow := pal[3]
	shadow.A = 0xB0
	fillRect(dst, 0, 0, sw, sh, color.NRGBA{shadow.R, shadow.G, shadow.B, shadow.A})
	fillRect(dst, x-scale*2, y-scale*2, w+scale*4, h+scale*4, pal[3])
	fillRect(dst, x, y, w, h, pal[0])

	ty := y + pad
	drawText(dst, v.title, x+pad, ty, scale, pal[3])
	ty += lineH * 1.5
	if len(v.tabs) > 0 {
		tx := x + pad
		for _, t := range v.tabs {
			label := " " + t.label + " "
			tw := advance(label) * scale
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
	for i, s := range l.items {
		fg := pal[3]
		if v.disabled[l.first+i] {
			fg = pal[1] // faded: not available
		}
		if i == l.selected {
			fillRect(dst, x+pad/2, ty-2*scale, w-pad, lineH, pal[2])
			fg = pal[0]
			s = "> " + s
		} else {
			s = "  " + s
		}
		drawText(dst, s, x+pad, ty, scale, fg)
		// More entries above or below: a small arrow at the right.
		if (i == 0 && l.above) || (i == len(l.items)-1 && l.below) {
			drawScrollArrow(dst, x+w-pad/2-5*scale, ty+3*scale, scale, i == 0, fg)
		}
		ty += lineH
	}
	for _, line := range l.footer {
		drawText(dst, line, x+pad, ty+lineH*0.5, scale, pal[2])
		ty += lineH
	}
}

// drawScrollArrow draws a small triangle pointing up or down, 5 pixels
// wide at scale 1, from (x, y).
func drawScrollArrow(dst *ebiten.Image, x, y, scale float64, up bool, c color.Color) {
	for row := range 3 {
		r := float64(row)
		if up {
			r = 2 - r
		}
		fillRect(dst, x+r*scale, y+float64(row)*scale+2*scale, (5-2*r)*scale, scale, c)
	}
}
