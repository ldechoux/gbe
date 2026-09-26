// Package ui is the Ebitengine frontend: window, input, audio and menu.
package ui

import (
	"image/color"
	"log"
	"math"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"gbe/internal/gb"
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
	ConfigPath string // "" for DefaultConfigPath()
	Scale      int    // 0 to use the configured scale
}

// Game implements ebiten.Game.
type Game struct {
	gb       *gb.GameBoy
	cfg      *Config
	cfgPath  string
	savePath string

	lcd  *ebiten.Image
	pix  []byte
	menu menu

	stream *audioStream
	player *audio.Player

	ignoredKeys map[ebiten.Key]bool // held when the menu closed
	frame       int
	quit        bool
}

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

	g := &Game{
		gb:          opts.GameBoy,
		cfg:         cfg,
		cfgPath:     cfgPath,
		savePath:    opts.SavePath,
		lcd:         ebiten.NewImage(gb.ScreenWidth, gb.ScreenHeight),
		pix:         make([]byte, gb.ScreenWidth*gb.ScreenHeight*4),
		stream:      &audioStream{},
		ignoredKeys: map[ebiten.Key]bool{},
	}

	ctx := audio.NewContext(sampleRate)
	g.player, err = ctx.NewPlayer(g.stream)
	if err != nil {
		return err
	}
	g.player.SetBufferSize(40 * time.Millisecond)
	g.player.SetVolume(cfg.Volume)
	g.player.Play()

	title := "gbe"
	if opts.Title != "" {
		title += " - " + opts.Title
	}
	ebiten.SetWindowTitle(title)
	ebiten.SetWindowSize(gb.ScreenWidth*cfg.Scale, gb.ScreenHeight*cfg.Scale)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(60)

	err = ebiten.RunGame(g)
	g.saveBattery()
	return err
}

func (g *Game) palette() *Palette { return &Palettes[paletteIndex(g.cfg.Palette)] }

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

func (g *Game) cyclePalette(delta int) {
	i := (paletteIndex(g.cfg.Palette) + delta + len(Palettes)) % len(Palettes)
	g.cfg.Palette = Palettes[i].ID
	g.saveConfig()
}

func (g *Game) Update() error {
	if g.quit {
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	}
	if g.menu.open {
		g.menu.update(g)
		if !g.menu.open {
			// Keys used to leave the menu (e.g. Enter) must not reach the game.
			for _, k := range inpututil.AppendPressedKeys(nil) {
				g.ignoredKeys[k] = true
			}
		}
		return nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
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
	for _, b := range gb.Buttons {
		k := g.cfg.Key(b)
		g.gb.SetButton(b, ebiten.IsKeyPressed(k) && !g.ignoredKeys[k])
	}

	g.gb.APU.SetSampleRate(emulatedRate(g.stream.buffered()))
	g.gb.RunFrame()
	g.stream.push(g.gb.APU.DrainSamples())

	g.frame++
	if g.frame%saveEveryFrame == 0 {
		g.saveBattery()
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	pal := g.palette()
	for i, s := range g.gb.Framebuffer() {
		c := pal.Colors[s]
		g.pix[i*4], g.pix[i*4+1], g.pix[i*4+2], g.pix[i*4+3] = c.R, c.G, c.B, 0xFF
	}
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
}

func (g *Game) Layout(w, h int) (int, int) { return w, h }

// LayoutF renders at the physical resolution so HiDPI screens stay sharp.
func (g *Game) LayoutF(w, h float64) (float64, float64) {
	s := ebiten.Monitor().DeviceScaleFactor()
	return w * s, h * s
}
