// Package ui is the Ebitengine frontend: window, input, audio and menu.
package ui

import (
	"errors"
	"fmt"
	"image/color"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/i18n"
)

const (
	maxScale       = 8
	saveEveryFrame = 5 * 60
)

// Options configures Run.
type Options struct {
	GameBoy    *gb.GameBoy
	Title      string
	SavePath   string // battery save file
	StatePath  string // save state, written on exit and offered on launch
	ConfigPath string // "" for DefaultConfigPath()
	Scale      int    // 0 to use the configured scale
	// ScreenshotDir receives the PNG captures ("" for DefaultScreenshotDir()).
	ScreenshotDir string
	// FreshStart, when set, builds the console to use if the player starts
	// over: GameBoy then runs in the hardware mode of the save state, so it
	// can be resumed, and FreshStart in the preferred one (see cmd/gbe).
	FreshStart func() (*gb.GameBoy, error)
}

// Game implements ebiten.Game.
type Game struct {
	gb        *gb.GameBoy
	cfg       *Config
	cfgPath   string
	savePath  string
	statePath string
	stateTime time.Time // modification time of the save state, if any
	started   bool      // false while the resume prompt is shown
	title     string
	shotDir   string

	freshStart func() (*gb.GameBoy, error) // see Options

	lcd  *ebiten.Image
	pix  []byte
	menu menu

	stream *audioStream
	player *audio.Player

	ignoredKeys map[ebiten.Key]bool // held when the menu closed
	ignoredPad  map[padButton]bool  // same for gamepad buttons

	pads    padReader
	stick   stickNav
	actions menuActions // this tick's menu actions (keyboard + gamepad)
	frame   int
	fps     fpsCounter
	quit    bool

	rewind     rewinder
	rewindTick int      // ticks spent rewinding, which steps back every other tick
	speed      playMode // shown on screen while not normal

	toast      string // short on-screen notification
	toastUntil int64  // tick at which the toast disappears
}

// playMode is how the emulation advances on this tick.
type playMode int

const (
	playNormal playMode = iota
	playFast
	playRewind
)

// Run opens the window and runs the emulator until it is closed.
func Run(opts Options) error {
	cfgPath := opts.ConfigPath
	if cfgPath == "" {
		cfgPath = DefaultConfigPath()
	}
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		log.Printf("config %s: %v (using defaults)", cfgPath, err)
	}
	if opts.Scale > 0 {
		cfg.Scale = min(opts.Scale, maxScale)
	}

	if opts.ScreenshotDir == "" {
		opts.ScreenshotDir = DefaultScreenshotDir()
	}
	g := &Game{
		gb:          opts.GameBoy,
		cfg:         cfg,
		cfgPath:     cfgPath,
		savePath:    opts.SavePath,
		statePath:   opts.StatePath,
		title:       opts.Title,
		shotDir:     opts.ScreenshotDir,
		freshStart:  opts.FreshStart,
		lcd:         ebiten.NewImage(gb.ScreenWidth, gb.ScreenHeight),
		pix:         make([]byte, gb.ScreenWidth*gb.ScreenHeight*4),
		stream:      &audioStream{},
		ignoredKeys: map[ebiten.Key]bool{},
		ignoredPad:  map[padButton]bool{},
		pads:        &ebitenPads{},
	}

	ctx := audio.NewContext(sampleRate)
	g.player, err = ctx.NewPlayer(g.stream)
	if err != nil {
		return err
	}
	g.player.SetBufferSize(40 * time.Millisecond)
	g.player.SetVolume(cfg.Volume)
	g.player.Play()

	if fi, err := os.Stat(g.statePath); g.statePath != "" && err == nil {
		g.stateTime = fi.ModTime()
		g.menu.showStart()
	} else {
		g.started = true
	}

	// Updated with the frame rate every fpsRefreshInterval (see Update).
	ebiten.SetWindowTitle(windowTitle(g.tr(), opts.Title, 0, !g.started))
	ebiten.SetWindowSize(gb.ScreenWidth*cfg.Scale, gb.ScreenHeight*cfg.Scale)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(60)

	err = ebiten.RunGame(g)
	if g.started {
		g.saveState() // so the next launch can resume
	}
	g.saveBattery()
	return err
}

func (g *Game) palette() *Palette { return &Palettes[paletteIndex(g.cfg.Palette)] }

// menuPalette colors the menu and the notifications: the selected palette
// in DMG mode, black and white for Game Boy Color games, whose screen has
// no palette to match.
func (g *Game) menuPalette() *Palette {
	if g.colorMode() {
		return &Palettes[paletteIndex("grey")]
	}
	return g.palette()
}

// tr returns the language of the user interface.
func (g *Game) tr() *i18n.Locale { return i18n.Get(g.cfg.Language) }

func (g *Game) cycleLanguage(delta int) {
	langs := i18n.Languages()
	i := slices.IndexFunc(langs, func(l *i18n.Locale) bool { return l.Code == g.tr().Code })
	g.cfg.Language = langs[(i+delta+len(langs))%len(langs)].Code
	g.saveConfig()
}

func (g *Game) saveConfig() {
	if err := g.cfg.Save(g.cfgPath); err != nil {
		log.Printf("saving config: %v", err)
	}
}

func (g *Game) saveBattery() {
	if !g.gb.Cart.Dirty() || g.savePath == "" {
		return
	}
	if err := os.WriteFile(g.savePath, g.gb.Cart.SaveData(), 0o644); err != nil {
		log.Printf("saving %s: %v", g.savePath, err)
	}
}

// colorMode reports whether a Game Boy Color game is running.
func (g *Game) colorMode() bool { return g.gb != nil && g.gb.IsCGB() }

// compat reports whether a DMG game runs colorized on a Game Boy Color.
func (g *Game) compat() bool { return g.gb != nil && g.gb.Compat() }

// compatChoice is the palette chosen for this colorized DMG game, or
// gb.CompatAuto.
func (g *Game) compatChoice() int { return compatPaletteIndex(g.cfg.CompatPalettes[g.title]) }

// applyCompatChoice loads the palette chosen for a colorized DMG game, if
// any. It runs every frame, so the choice also replaces the palette of the
// boot ROM, of a loaded state or of a rewind; the game itself cannot change
// it. With Auto, the palette is left alone, which keeps the one picked by
// holding buttons during the boot ROM logo.
func (g *Game) applyCompatChoice() {
	if c := g.compatChoice(); c != gb.CompatAuto {
		g.gb.SetCompatPalette(c)
	}
}

// cyclePalette changes the DMG palette, or toggles the color correction in
// Game Boy Color mode, where palettes do not apply. A colorized DMG game
// goes through the palettes of the CGB boot ROM.
func (g *Game) cyclePalette(delta int) {
	switch {
	case g.compat():
		n := gb.CompatKeyPalettes + 1 // Auto, then the button combinations
		i := (g.compatChoice()+1+delta%n+n)%n - 1
		if i == gb.CompatAuto {
			delete(g.cfg.CompatPalettes, g.title)
		} else {
			g.cfg.CompatPalettes[g.title] = compatPaletteID(i)
		}
		g.gb.SetCompatPalette(i)
	case g.colorMode():
		g.cfg.ColorCorrection = !g.cfg.ColorCorrection
	default:
		i := (paletteIndex(g.cfg.Palette) + delta + len(Palettes)) % len(Palettes)
		g.cfg.Palette = Palettes[i].ID
	}
	g.saveConfig()
}

// saveState writes the save state atomically (temp file + rename).
func (g *Game) saveState() {
	if g.statePath == "" {
		return
	}
	tmp := g.statePath + ".tmp"
	err := os.WriteFile(tmp, g.gb.SaveState(), 0o644)
	if err == nil {
		err = os.Rename(tmp, g.statePath)
	}
	if err != nil {
		log.Printf("saving state: %v", err)
		g.notify(g.tr().T("toast.save_failed", err))
		return
	}
	g.stateTime = time.Now()
	g.notify(g.tr().T("toast.state_saved"))
}

// loadState restores the save state; on failure the game keeps running.
// It reports whether the state was loaded.
func (g *Game) loadState() bool {
	data, err := os.ReadFile(g.statePath)
	if err == nil {
		err = g.gb.LoadState(data)
	}
	if err == nil {
		g.rewind.clear() // that history belongs to the abandoned game
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		g.notify(g.tr().T("toast.no_state"))
	case err != nil:
		log.Printf("loading state: %v", err)
		g.notify(g.tr().T("toast.state_unreadable", err))
	default:
		g.notify(g.tr().T("toast.state_restored"))
	}
	return err == nil
}

// startFresh replaces the console by one in the preferred hardware mode,
// when the save state offered at launch was made in the other one.
func (g *Game) startFresh() {
	if g.freshStart == nil {
		return
	}
	console, err := g.freshStart()
	if err != nil {
		log.Printf("restarting: %v", err)
		g.notify(g.tr().T("toast.restart_failed", err))
		return
	}
	g.gb = console
	g.rewind.clear()
}

// screenshot saves the current frame, scaled like the window, as a PNG.
func (g *Game) screenshot() {
	img := Screenshot(g.gb, g.palette(), g.cfg.ColorCorrection, g.cfg.Scale)
	path, err := saveScreenshot(g.shotDir, g.title, img, time.Now())
	if err != nil {
		log.Printf("screenshot: %v", err)
		g.notify(g.tr().T("toast.screenshot_failed", err))
		return
	}
	log.Printf("screenshot saved to %s", path)
	g.notify(g.tr().T("toast.screenshot", filepath.Base(path)))
}

func (g *Game) notify(msg string) {
	g.toast = msg
	g.toastUntil = ebiten.Tick() + 2*60
}

// ignoreHeldInputs keeps the keys and buttons used to leave the menu (Enter,
// A, Start+Select...) from reaching the game until they are released.
func (g *Game) ignoreHeldInputs() {
	for _, k := range inpututil.AppendPressedKeys(nil) {
		g.ignoredKeys[k] = true
	}
	for b := range padButtonNames {
		if padHeld(g.pads, b) {
			g.ignoredPad[b] = true
		}
	}
}

func (g *Game) Update() error {
	if g.quit {
		return ebiten.Termination
	}
	if g.fps.update(time.Now()) {
		ebiten.SetWindowTitle(windowTitle(g.tr(), g.title, g.fps.fps, g.menu.open))
	}
	if g.cfg.Screenshot.justPressed() {
		g.screenshot()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	}
	g.actions = keyboardActions().or(padActions(g.pads, g.cfg.Gamepad, &g.stick))
	if g.menu.open {
		g.menu.update(g)
		if !g.menu.open {
			g.ignoreHeldInputs()
		}
		return nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || g.actions.toggle {
		g.menu.show()
		return nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyP) && !g.cfg.bound(ebiten.KeyP) {
		g.cyclePalette(1)
	}

	for k := range g.ignoredKeys {
		if !ebiten.IsKeyPressed(k) {
			delete(g.ignoredKeys, k)
		}
	}
	for b := range g.ignoredPad {
		if !padHeld(g.pads, b) {
			delete(g.ignoredPad, b)
		}
	}
	pad := padGameButtons(g.pads, g.cfg.Gamepad, g.ignoredPad)
	for _, b := range gb.Buttons {
		k := g.cfg.Key(b)
		g.gb.SetButton(b, (ebiten.IsKeyPressed(k) && !g.ignoredKeys[k]) || pad[b])
	}
	g.applyCompatChoice()
	g.advance(g.held(actionFastForward), g.held(actionRewind))

	g.frame++
	if g.frame%saveEveryFrame == 0 {
		g.saveBattery()
	}
	return nil
}

// held reports whether the key or gamepad button of an action is held.
func (g *Game) held(action string) bool {
	k, b := g.cfg.Keys[action], g.cfg.Gamepad[action]
	return (ebiten.IsKeyPressed(k) && !g.ignoredKeys[k]) || (!g.ignoredPad[b] && padHeld(g.pads, b))
}

// advance runs this tick's emulation: one frame, several while fast
// forwarding, or a step back in time while rewinding (which wins when both
// are held).
func (g *Game) advance(fast, rewind bool) {
	if rewind {
		g.speed = playRewind
		// Every other tick: twice as fast as the game went forward. No audio
		// is queued, so the stream repeats its last frame, which is silent.
		if g.rewindTick%2 == 0 {
			g.rewind.step(g.gb)
		}
		g.rewindTick++
		return
	}
	g.rewindTick = 0
	n := 1
	g.speed = playNormal
	if fast {
		n, g.speed = g.cfg.FastForwardSpeed, playFast
	}
	// The APU generates n times fewer samples per emulated frame, so a tick
	// still queues one frame's worth of audio: the sound plays faster
	// instead of piling up.
	g.gb.APU.SetSampleRate(emulatedRate(g.stream.buffered()) / float64(n))
	for range n {
		g.gb.RunFrame()
		g.fps.frame()
		g.rewind.record(g.gb)
	}
	g.stream.push(g.gb.APU.DrainSamples())
}

// speedBadge is the indicator shown while fast forwarding or rewinding.
func (g *Game) speedBadge() string {
	switch g.speed {
	case playFast:
		return fmt.Sprintf(">> x%d", g.cfg.FastForwardSpeed)
	case playRewind:
		return "<<"
	}
	return ""
}

func (g *Game) Draw(screen *ebiten.Image) {
	frameRGBA(g.pix, g.gb, g.palette(), g.cfg.ColorCorrection)
	g.lcd.WritePixels(g.pix)

	screen.Fill(color.RGBA{0x10, 0x10, 0x10, 0xFF})
	sw, sh := float64(screen.Bounds().Dx()), float64(screen.Bounds().Dy())
	scale := math.Min(sw/gb.ScreenWidth, sh/gb.ScreenHeight)
	if scale >= 1 {
		scale = math.Floor(scale) // crisp integer scaling when possible
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate((sw-gb.ScreenWidth*scale)/2, (sh-gb.ScreenHeight*scale)/2)
	screen.DrawImage(g.lcd, op)

	if g.menu.open {
		g.menu.draw(screen, g)
	}
	if g.toast != "" && ebiten.Tick() < g.toastUntil {
		drawToast(screen, g.toast, g.menuPalette())
	}
	if badge := g.speedBadge(); badge != "" && !g.menu.open {
		drawBadge(screen, badge, g.menuPalette())
	}
}

func (g *Game) Layout(w, h int) (int, int) { return w, h }

// LayoutF renders at the physical resolution so HiDPI screens stay sharp.
func (g *Game) LayoutF(w, h float64) (float64, float64) {
	s := ebiten.Monitor().DeviceScaleFactor()
	return w * s, h * s
}
