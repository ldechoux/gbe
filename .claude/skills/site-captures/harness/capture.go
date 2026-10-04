package ui

// Temporary harness of the site-captures skill (.claude/skills/site-captures):
// capture.sh copies it to internal/ui/zz_capture.go and removes it afterwards.
// It must never be committed there.

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/rom"
	"github.com/ldechoux/gbe/internal/ui/scaler"
)

// CaptureOptions describes one console's captures.
type CaptureOptions struct {
	Dir      string // output directory
	TitleAt  int    // frame at which the title screen shows
	StartAt  int    // frame at which Start is pressed to skip an intro, 0 for never
	Scale    int    // window scale of the captures
	Filter   string // display filter
	Language string // user interface language
}

type captureGame struct {
	*Game
	opts CaptureOptions
	step int
}

// save writes what the screenshot hotkey would: renderFrame at the window
// scale, menu included when it is open.
func (c *captureGame) save(name string) {
	f, err := os.Create(c.opts.Dir + "/" + name + ".png")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, c.renderFrame(c.cfg.Scale)); err != nil {
		panic(err)
	}
}

func (c *captureGame) Update() error {
	switch {
	case c.step == 0: // at launch, before the first frame: the resume prompt
		c.started = false
		c.stateTime = time.Now()
		c.menu.showStart()
		c.save("save_state_resume")
		c.menu.open, c.started = false, true
		c.step++
	case c.frame < c.opts.TitleAt:
		start := c.opts.StartAt > 0 && c.frame >= c.opts.StartAt && c.frame < c.opts.StartAt+10
		c.gb.SetButton(gb.ButtonStart, start)
		c.gb.RunFrame()
		c.frame++
	case c.step == 1:
		c.save("game_title")
		c.menu.show() // the pause menu, cursor on Resume
		c.step++
	case c.step == 2:
		c.save("menu")
		return ebiten.Termination
	}
	return nil
}

// Capture runs console and saves save_state_resume.png, game_title.png and
// menu.png in opts.Dir.
func Capture(console *gb.GameBoy, opts CaptureOptions) error {
	cfg := DefaultConfig() // default palette (DMG vert) and color correction
	cfg.Scale = opts.Scale
	cfg.Filter = opts.Filter
	cfg.Language = opts.Language
	// A game played before: the Recent games entry of the menu is enabled,
	// as for any player but the very first time.
	cfg.Recent = []RecentGame{{Path: opts.Dir + "/played.gb", Title: "PLAYED"}}
	g := &Game{gb: console, cfg: cfg, cfgPath: opts.Dir + "/config.json", title: "capture", shotDir: opts.Dir,
		lcd: ebiten.NewImage(gb.ScreenWidth, gb.ScreenHeight), stream: &audioStream{},
		ignoredKeys: map[ebiten.Key]bool{}, ignoredPad: map[padButton]bool{}, pads: &ebitenPads{}, started: true}
	ebiten.SetWindowSize(gb.ScreenWidth*4, gb.ScreenHeight*4)
	return ebiten.RunGame(&captureGame{Game: g, opts: opts})
}

// dropCapture saves the drop screen, shown when gbe runs without a ROM.
type dropCapture struct {
	*Game
	opts CaptureOptions
}

func (d *dropCapture) Update() error {
	f, err := os.Create(d.opts.Dir + "/drop_screen.png")
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, d.renderFrame(d.cfg.Scale)); err != nil {
		return err
	}
	return ebiten.Termination
}

// CaptureDrop saves drop_screen.png in opts.Dir: the drop screen, its recent
// games being roms (the first one selected), named by their header title
// as gbe shows them. The palette is the default one; there is no filter
// without a game.
func CaptureDrop(roms []string, opts CaptureOptions) error {
	cfg := DefaultConfig()
	cfg.Scale = opts.Scale
	cfg.Language = opts.Language
	for _, path := range roms {
		game, err := rom.Open(path, rom.Options{BootROM: "none", Fresh: true})
		if err != nil {
			return err
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		cfg.Recent = append(cfg.Recent, RecentGame{Path: abs, Title: game.Title})
	}
	g := &Game{cfg: cfg, cfgPath: opts.Dir + "/config.json", shotDir: opts.Dir,
		lcd: ebiten.NewImage(gb.ScreenWidth, gb.ScreenHeight), lcdPrev: ebiten.NewImage(gb.ScreenWidth, gb.ScreenHeight),
		stream: &audioStream{}, ignoredKeys: map[ebiten.Key]bool{}, ignoredPad: map[padButton]bool{},
		pads: &ebitenPads{}, monitors: &ebitenMonitors{}}
	ebiten.SetWindowSize(gb.ScreenWidth*4, gb.ScreenHeight*4)
	return ebiten.RunGame(&dropCapture{Game: g, opts: opts})
}

// filterCapture renders an image through every display filter.
type filterCapture struct {
	src   image.Image
	color bool
	gap   color.RGBA
	w, h  int
	dir   string
	err   error
}

func (f *filterCapture) Update() error {
	src := ebiten.NewImageFromImage(f.src)
	for _, filter := range scaler.Filters {
		dst := ebiten.NewImage(f.w, f.h)
		var p scaler.Pipeline
		p.Draw(dst, &filter, scaler.Frame{Image: src, Changed: true, Color: f.color, Gap: f.gap})
		img := image.NewRGBA(image.Rect(0, 0, f.w, f.h))
		dst.ReadPixels(img.Pix)
		out, err := os.Create(fmt.Sprintf("%s/%s.png", f.dir, filter.ID))
		if err != nil {
			f.err = err
			return ebiten.Termination
		}
		err = png.Encode(out, img)
		out.Close()
		if err != nil {
			f.err = err
			return ebiten.Termination
		}
	}
	return ebiten.Termination
}

func (f *filterCapture) Draw(*ebiten.Image)         {}
func (f *filterCapture) Layout(w, h int) (int, int) { return w, h }

// CaptureFilters saves <filter id>.png in dir for every display filter: src
// (a 160x144 frame) drawn at w x h, as the window draws it. color is the
// Game Boy Color mode of the LCD filter, gap the color between the dots of
// its DMG grid.
func CaptureFilters(src image.Image, colorMode bool, gap color.RGBA, w, h int, dir string) error {
	ebiten.SetWindowSize(gb.ScreenWidth*2, gb.ScreenHeight*2)
	f := &filterCapture{src: src, color: colorMode, gap: gap, w: w, h: h, dir: dir}
	if err := ebiten.RunGame(f); err != nil {
		return err
	}
	return f.err
}
