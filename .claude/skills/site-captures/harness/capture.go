package ui

// Temporary harness of the site-captures skill (.claude/skills/site-captures):
// capture.sh copies it to internal/ui/zz_capture.go and removes it afterwards.
// It must never be committed there.

import (
	"image/png"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/gb"
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
	g := &Game{gb: console, cfg: cfg, cfgPath: opts.Dir + "/config.json", title: "capture", shotDir: opts.Dir,
		lcd: ebiten.NewImage(gb.ScreenWidth, gb.ScreenHeight), stream: &audioStream{},
		ignoredKeys: map[ebiten.Key]bool{}, ignoredPad: map[padButton]bool{}, pads: &ebitenPads{}, started: true}
	ebiten.SetWindowSize(gb.ScreenWidth*4, gb.ScreenHeight*4)
	return ebiten.RunGame(&captureGame{Game: g, opts: opts})
}
