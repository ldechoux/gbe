// Package ui is the Ebitengine frontend: window, input, audio and menu.
package ui

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/ldechoux/gbe/assets/icon"
	"github.com/ldechoux/gbe/internal/audiofx"
	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/i18n"
	"github.com/ldechoux/gbe/internal/rom"
	"github.com/ldechoux/gbe/internal/ui/scaler"
)

const (
	maxScale       = 8
	saveEveryFrame = 5 * 60
)

// Options configures Run.
type Options struct {
	// Game runs at launch; nil shows a screen asking for a ROM to be
	// dropped on the window. Its save state is written on exit and offered
	// on launch.
	Game *rom.Game
	// LaunchError, with Game nil, is why the game at LaunchPath, given at
	// launch, could not be opened: the drop screen tells it, as for a game
	// dropped on the window.
	LaunchPath  string
	LaunchError error
	// BootROMs is where the games opened by Open search their boot ROM: the
	// folder chosen in the menu is set in it.
	BootROMs   *rom.BootROMSearch
	ConfigPath string // "" for DefaultConfigPath()
	Scale      int    // 0 to use the configured scale
	Filter     string // "" to use the configured filter (see scaler.Filters)
	// Stereo and AudioFilter override the configured ones ("" to keep
	// them; see audiofx.StereoModes and audiofx.Filters).
	Stereo, AudioFilter string
	// ScreenshotDir receives the PNG captures ("" for DefaultScreenshotDir()).
	ScreenshotDir string
	// Open opens a game dropped on the window, colorize being the setting
	// of the menu (see rom.Options).
	Open func(path string, colorize bool) (*rom.Game, error)
	// RumbleRecord, when set, is the folder where stretches of play are
	// recorded to tune the rumble detector (see rumbleRecorder).
	RumbleRecord string
	// RumbleDebug shows what the rumble detector hears, at the top of the
	// screen.
	RumbleDebug bool
	// RumbleTune, when set, is a tuning session to run instead of a game
	// (see tuner).
	RumbleTune string
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
	romPath   string // the file the game was opened from
	shotDir   string

	// autoModel is set when the hardware was left to auto: the settings
	// then decide it (a DMG game is colorized or not), and newConsole builds
	// the console again when they change. gb may still run in the other
	// mode, that of its save state, so it can be resumed (see rom.Open).
	autoModel  bool
	newConsole func(gb.Model) (*gb.GameBoy, error)

	open    func(path string, colorize bool) (*rom.Game, error) // see Options
	pending *rom.Game                                           // dropped, waiting for the player's choice (pageSwitch)

	recentCursor int // the recent game selected on the drop screen

	lcd      *ebiten.Image
	lcdFrame lcdFrame        // the frame in lcd
	video    scaler.Pipeline // draws lcd on the screen
	menu     menu

	// The frame the console produced just before its current one, which
	// ghosting blends with it (see keepPrevious), and the parity of the
	// current one.
	prevShades [screenPixels]byte
	prevColors [screenPixels]uint16
	prevColor  bool
	lcdPrev    *ebiten.Image
	prevFrame  lcdFrame // the frame in lcdPrev
	oddFrame   bool

	stream *audioStream
	player *audio.Player
	fx     *audiofx.Chain // the stereo mode and the filter of the settings
	sfx    soundPlayer    // the sounds of the menus (none in the tests)

	// The boot ROM folder: the search the games use, the folder dialog
	// (none in the tests) and its outcome, and whether the running game
	// needs a reset to use a new folder.
	bootROMs       *rom.BootROMSearch
	picker         folderPicker
	picks          chan folderPick
	picking        bool
	bootROMPending bool

	ignoredKeys map[ebiten.Key]bool // held when the menu closed
	ignoredPad  map[padButton]bool  // same for gamepad buttons

	pads padReader
	// monitors lets the display page move the window to a TV (nil in tests
	// without monitors).
	monitors monitorSet
	stick    stickNav
	actions  menuActions // this tick's menu actions (keyboard + gamepad)
	frame    int
	fps      fpsCounter
	quit     bool

	// The window is left alone until screenBusyUntil (a tick), while it
	// switches to or from the full screen (see screen.go). Then
	// screenCheck reads the mode it ended in, and sizePending applies the
	// scale chosen meanwhile.
	screenBusyUntil int64
	screenCheck     bool
	sizePending     bool
	// fullscreenPending asks for cfg.Fullscreen at the tick fullscreenAt.
	fullscreenAt      int64
	fullscreenPending bool
	// staleKeys were held during a switch, which may have lost their
	// release: they are ignored until it is seen (see screen.go).
	staleKeys map[ebiten.Key]bool

	rewind rewinder
	rumble rumbler
	// down are the buttons held this tick, recorder records them (nil
	// unless -rumble-record), and rumbleDebug shows what the rumble
	// detector hears.
	down        [8]bool
	recorder    *rumbleRecorder
	tune        *tuner // nil unless -rumble-tune
	rumbleDebug bool
	lastNote    gb.RumbleNote // of channel 1 or 4, for rumbleDebug
	rewindTick  int           // ticks spent rewinding, which steps back every other tick
	speed       playMode      // shown on screen while not normal

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
	if id, ok := scaler.ID(opts.Filter); ok {
		cfg.Filter = id
	}
	if id, ok := audiofx.ID(audiofx.StereoModes, opts.Stereo); ok {
		cfg.Stereo = id
	}
	if id, ok := audiofx.ID(audiofx.Filters, opts.AudioFilter); ok {
		cfg.AudioFilter = id
	}

	if opts.ScreenshotDir == "" {
		opts.ScreenshotDir = DefaultScreenshotDir()
	}
	g := &Game{
		cfg:         cfg,
		cfgPath:     cfgPath,
		shotDir:     opts.ScreenshotDir,
		rumbleDebug: opts.RumbleDebug,
		open:        opts.Open,
		lcd:         ebiten.NewImage(gb.ScreenWidth, gb.ScreenHeight),
		lcdPrev:     ebiten.NewImage(gb.ScreenWidth, gb.ScreenHeight),
		stream:      &audioStream{},
		fx:          audiofx.New(sampleRate, cfg.Stereo, cfg.AudioFilter),
		ignoredKeys: map[ebiten.Key]bool{},
		ignoredPad:  map[padButton]bool{},
		pads:        &ebitenPads{},
		monitors:    &ebitenMonitors{},
	}

	if opts.RumbleRecord != "" {
		g.recorder = &rumbleRecorder{dir: opts.RumbleRecord}
	}
	if opts.RumbleTune != "" {
		if g.tune, err = loadTuner(opts.RumbleTune); err != nil {
			return err
		}
		cfg.Vibration = vibrationAll // for this session only: the config is not saved
	}

	ctx := audio.NewContext(sampleRate)
	g.player, err = ctx.NewPlayer(g.stream)
	if err != nil {
		return err
	}
	g.player.SetBufferSize(40 * time.Millisecond)
	g.player.SetVolume(cfg.Volume)
	g.player.Play()
	g.sfx = newEbitenSounds(ctx)
	g.bootROMs, g.picker = opts.BootROMs, systemPicker{}
	if g.bootROMs != nil {
		g.bootROMs.Dir = cfg.BootROMDir
	}

	// With a game, the title gets its name, and the frame rate every
	// fpsRefreshInterval (see Update).
	ebiten.SetWindowTitle("gbe")
	ebiten.SetWindowIcon(icon.Window())
	g.pruneRecent()
	if opts.Game != nil {
		g.startGame(opts.Game)
	} else if opts.LaunchError != nil {
		g.notifyLong(dropError(g.tr(), filepath.Base(opts.LaunchPath), opts.LaunchError))
	}
	ebiten.SetWindowSize(gb.ScreenWidth*cfg.Scale, gb.ScreenHeight*cfg.Scale)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	g.restoreMonitor()
	ebiten.SetFullscreen(cfg.Fullscreen)
	if cfg.Fullscreen {
		g.switchingScreen()
	}
	ebiten.SetTPS(60)

	err = ebiten.RunGame(g)
	g.closeGame() // so the next launch can resume
	return err
}

func (g *Game) palette() *Palette { return &Palettes[paletteIndex(g.cfg.Palette)] }

func (g *Game) filter() *scaler.Filter { return &scaler.Filters[scaler.Index(g.cfg.Filter)] }

func (g *Game) cycleFilter(delta int) {
	n := len(scaler.Filters)
	g.cfg.Filter = scaler.Filters[(scaler.Index(g.cfg.Filter)+delta%n+n)%n].ID
	g.saveConfig()
}

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
	if g.gb == nil || !g.gb.Cart.Dirty() || g.savePath == "" {
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
		g.keepPrevious() // and so does the previous frame
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

// preferredModel is the hardware the settings ask for: the Game Boy Color
// for the games that support it, and for DMG games unless colorization is
// off.
func (g *Game) preferredModel() gb.Model {
	if g.gb.Cart.ColorSupported() || g.cfg.ColorizeDMG {
		return gb.ModelCGB
	}
	return gb.ModelDMG
}

// modePending reports whether the console runs in another hardware mode than
// the settings ask for, until restart applies them: the setting changed, or
// the game was resumed from a save state made in the other mode.
func (g *Game) modePending() bool {
	if !g.autoModel || g.newConsole == nil || g.gb == nil {
		return false
	}
	return g.runningModel() != g.preferredModel()
}

// runningModel is the hardware the console runs as.
func (g *Game) runningModel() gb.Model {
	if g.gb.IsCGB() {
		return gb.ModelCGB
	}
	return gb.ModelDMG
}

// colorizable reports whether the colorization setting applies to the game:
// a DMG game, whose hardware was left to auto.
func (g *Game) colorizable() bool {
	return g.autoModel && g.gb != nil && !g.gb.Cart.ColorSupported()
}

// restart power-cycles the console, in the hardware mode the settings ask
// for. The cartridge, and so its battery RAM, is kept.
func (g *Game) restart() {
	g.rewind.clear()
	defer g.keepPrevious() // the previous frame belongs to the abandoned game
	model := g.preferredModel()
	if !g.modePending() {
		// A new boot ROM folder needs a new console, of the same model.
		if !g.bootROMPending || g.newConsole == nil {
			g.gb.Reset()
			return
		}
		model = g.runningModel()
	}
	g.bootROMPending = false
	console, err := g.newConsole(model)
	if err != nil {
		log.Printf("restarting: %v", err)
		g.notify(g.tr().T("toast.restart_failed", err))
		g.gb.Reset()
		return
	}
	g.gb = console
}

// screenshot saves the screen as a PNG at the scale of the window: the
// frame drawn with the filter and ghosting, and the menu when it is open
// (to show it, on the project site for instance), without the
// notifications.
func (g *Game) screenshot() {
	img := g.renderFrame(g.cfg.Scale)
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
		g.rumble.stop(g.pads)
		if g.recorder != nil {
			g.recorder.finish()
		}
		return ebiten.Termination
	}
	if fsys := ebiten.DroppedFiles(); fsys != nil {
		g.drop(fsys)
	}
	g.pollFolderPick()
	if g.fps.update(time.Now()) && g.gb != nil {
		ebiten.SetWindowTitle(windowTitle(g.tr(), g.title, g.fps.fps, g.menu.open))
	}
	if g.cfg.Screenshot.justPressed() {
		g.screenshot()
	}
	g.settleScreen()
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		g.toggleFullscreen()
	}
	g.actions = keyboardActions(g.staleKeys).or(padActions(g.pads, g.cfg.Gamepad, &g.stick))
	if g.tune != nil {
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			g.quit = true
		}
		g.tune.update(g)
		return nil
	}
	if g.menu.open {
		g.rumble.stop(g.pads) // the game is paused
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
		g.cyclePalette(1) // also colors the drop screen
	}
	if g.gb == nil {
		g.updateDropScreen() // waiting for a ROM, dropped or recent
		return nil
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
	for i, b := range gb.Buttons {
		k := g.cfg.Key(b)
		g.down[i] = (ebiten.IsKeyPressed(k) && !g.ignoredKeys[k]) || pad[b]
		g.gb.SetButton(b, g.down[i])
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
		g.rumble.stop(g.pads)
		g.speed = playRewind
		// Every other tick: twice as fast as the game went forward. No audio
		// is queued, so the stream repeats its last frame, which is silent.
		if g.rewindTick%2 == 0 {
			g.rewind.step(g.gb)
			g.keepPrevious() // no ghost of the frame stepped back from
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
	g.gb.GuessRumble(g.rumbleDebug || g.cfg.Vibration == vibrationAll && !g.gb.HasMotor())
	g.gb.TraceRumble(g.rumbleDebug)
	rumble := 0.0
	for i := range n {
		if i == n-1 {
			g.keepPrevious() // the frame before the one shown
		}
		if g.recorder != nil {
			g.recorder.begin(g.gb, g.title, g.romPath)
		}
		g.gb.RunFrame()
		if g.recorder != nil {
			g.recorder.frame(g.gb, g.down)
		}
		g.oddFrame = !g.oddFrame
		g.fps.frame()
		g.rewind.record(g.gb)
		rumble += g.frameRumble()
	}
	for _, n := range g.gb.DrainRumbleNotes() {
		if n.Channel == 1 || n.Channel == 4 {
			g.lastNote = n
		}
	}
	samples := g.gb.APU.DrainSamples()
	if g.fx != nil {
		g.fx.Process(samples) // in place: the APU fills the slice again next frame
	}
	g.stream.push(samples)
	g.rumble.update(g.pads, g.cfg.Vibration != vibrationOff, rumble/float64(n), float64(g.cfg.VibrationStrength)/100)
}

// frameRumble is how hard the gamepads shake for the last frame: with the
// motor of the cartridge, or, for the other games when the setting allows
// it, from the sounds of shocks.
func (g *Game) frameRumble() float64 {
	switch {
	case g.gb.HasMotor():
		return g.gb.Rumble()
	case g.cfg.Vibration == vibrationAll:
		return g.gb.GuessedRumble()
	}
	return 0
}

// keepPrevious keeps the current frame of the console as the previous one,
// which ghosting blends with the next. Kept in the emulation loop rather
// than at drawing time, the two frames are always consecutive, however many
// frames a tick runs (fast forward, or several updates per draw). Called
// out of it (rewind, load, reset), it drops the ghost of an unrelated
// frame: the previous frame is the current one.
func (g *Game) keepPrevious() {
	if g.gb == nil {
		return
	}
	g.prevShades, g.prevColors = *g.gb.Framebuffer(), *g.gb.ColorFramebuffer()
	g.prevColor = g.gb.IsCGB()
}

// previous is the frame kept by keepPrevious.
func (g *Game) previous() frameSource {
	return frameSource{g.prevColor, &g.prevShades, &g.prevColors}
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

// drawFrame draws the last frame of the console on dst, with the filter.
func (g *Game) drawFrame(dst *ebiten.Image) {
	if g.gb == nil {
		g.drawDropScreen(dst)
		return
	}
	pal, correct := g.palette(), g.cfg.ColorCorrection
	fr := scaler.Frame{
		Image: g.lcd,
		Ghost: ghostMode(g.cfg.Ghosting),
		Odd:   g.oddFrame,
		Color: g.colorMode(),
		Gap:   pal.Colors[0],
	}
	if fr.Changed = g.lcdFrame.update(consoleFrame(g.gb), pal, correct); fr.Changed {
		g.lcd.WritePixels(g.lcdFrame.pix)
	}
	if fr.Ghost != scaler.GhostOff {
		if fr.PrevChanged = g.prevFrame.update(g.previous(), pal, correct); fr.PrevChanged {
			g.lcdPrev.WritePixels(g.prevFrame.pix)
		}
		fr.Prev = g.lcdPrev
	}
	dst.Fill(color.RGBA{0x10, 0x10, 0x10, 0xFF})
	g.video.Draw(dst, g.filter(), fr)
}

// renderFrame returns the last frame of the console drawn with the filter,
// each Game Boy pixel taking scale x scale pixels, and the menu over it
// when it is open.
func (g *Game) renderFrame(scale int) *image.RGBA {
	scale = max(1, scale)
	img := image.NewRGBA(image.Rect(0, 0, gb.ScreenWidth*scale, gb.ScreenHeight*scale))
	dst := ebiten.NewImage(img.Rect.Dx(), img.Rect.Dy())
	defer dst.Deallocate()
	g.drawFrame(dst)
	if g.menu.open {
		g.menu.draw(dst, g)
	}
	dst.ReadPixels(img.Pix)
	return img
}

func (g *Game) Draw(screen *ebiten.Image) {
	g.drawFrame(screen)

	if g.menu.open {
		g.menu.draw(screen, g)
	}
	if g.toast != "" && ebiten.Tick() < g.toastUntil {
		drawToast(screen, g.toast, g.menuPalette())
	}
	if badge := g.speedBadge(); badge != "" && !g.menu.open {
		drawBadge(screen, badge, g.menuPalette())
	}
	if g.tune != nil {
		drawBox(screen, g.tune.text(), g.menuPalette(), false)
	}
	if g.rumbleDebug && g.gb != nil && !g.menu.open {
		drawBox(screen, rumbleDebugText(g.gb.GuessedRumble(), g.lastNote), g.menuPalette(), false)
	}
}

// rumbleDebugText tells what the rumble detector hears: how hard it
// shakes, and the last note of channel 1 or 4 it judged.
func rumbleDebugText(level float64, n gb.RumbleNote) string {
	bar := int(math.Round(level * 10))
	s := fmt.Sprintf("rumble [%s%s] %.2f", strings.Repeat("#", bar), strings.Repeat("-", 10-bar), level)
	if n.Channel != 0 {
		kind := "fx"
		if n.Music >= 0.5 {
			kind = "music"
		}
		s += fmt.Sprintf("\nch%d %02X %02X %02X %02X %s %.2f shock %.2f", n.Channel, n.Regs[0], n.Regs[1], n.Regs[2], n.Regs[3], kind, n.Music, n.Shock)
	}
	return s
}

func (g *Game) Layout(w, h int) (int, int) { return w, h }

// LayoutF renders at the physical resolution so HiDPI screens stay sharp.
func (g *Game) LayoutF(w, h float64) (float64, float64) {
	s := ebiten.Monitor().DeviceScaleFactor()
	return w * s, h * s
}
