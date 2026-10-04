// Command zz_capture is the temporary entry point of the site-captures skill
// (.claude/skills/site-captures): capture.sh copies it to zz_capture/main.go
// and removes it afterwards.
//
// Modes:
//   - default: the save_state_resume, game_title and menu scenes of a ROM,
//     through the real Game (ui.Capture);
//   - -drop: the drop screen, the ROMs given as arguments listed as recent
//     games (ui.CaptureDrop);
//   - -shot: one frame of a ROM after some frames, through ui.Screenshot
//     (palette, color correction, scale, colorization palette);
//   - -filters: a 160x144 PNG drawn through every display filter at a given
//     size (ui.CaptureFilters).
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/ui"
)

func main() {
	romPath := flag.String("rom", "", "ROM to run")
	model := flag.String("model", "gb", "gb or gbc")
	drop := flag.Bool("drop", false, "capture the drop screen instead, the ROMs given as arguments listed as recent games")
	shot := flag.Bool("shot", false, "save one frame of the ROM to -file (see -frames, -press, -palette, -correct, -compat)")
	filters := flag.Bool("filters", false, "draw the PNG -src through every display filter at -size, into -out")
	file := flag.String("file", "", "-shot: PNG to write")
	frames := flag.Int("frames", 0, "-shot: frames to run")
	press := flag.String("press", "", `-shot: buttons held, e.g. "start:200-210,right:300-360" (frames, end excluded)`)
	palette := flag.String("palette", "dmg", "-shot: DMG palette (ui.Palettes ID)")
	correct := flag.Bool("correct", false, "-shot: Game Boy Color color correction")
	compat := flag.Int("compat", gb.CompatAuto, "-shot: palette of a DMG game colorized on a Game Boy Color: -1 auto, else the button combination (gb.compatKeyCombos order)")
	src := flag.String("src", "", "-filters: the 160x144 PNG")
	size := flag.String("size", "1280x1152", "-filters: size of the renders")
	gap := flag.String("gap", "9BBC0F", "-filters: color between the dots of the DMG LCD grid")
	var opts ui.CaptureOptions
	flag.StringVar(&opts.Dir, "out", "", "output directory")
	flag.IntVar(&opts.TitleAt, "title-at", 200, "frame at which the title screen shows")
	flag.IntVar(&opts.StartAt, "start-at", 0, "frame at which Start is pressed, 0 for never")
	flag.IntVar(&opts.Scale, "scale", 8, "window scale")
	flag.StringVar(&opts.Filter, "filter", "lcd", "display filter")
	flag.StringVar(&opts.Language, "lang", "fr", "user interface language")
	flag.Parse()

	var err error
	switch {
	case *drop:
		err = ui.CaptureDrop(flag.Args(), opts)
	case *shot:
		err = saveShot(*romPath, *model, *frames, *press, *palette, *correct, *compat, opts.Scale, *file)
	case *filters:
		err = renderFilters(*src, *model == "gbc", *gap, *size, opts.Dir)
	default:
		var console *gb.GameBoy
		if console, err = newConsole(*romPath, *model); err == nil {
			err = ui.Capture(console, opts)
		}
	}
	if err != nil {
		log.Fatal(err)
	}
}

// newConsole runs the ROM at path on a DMG ("gb") or a Game Boy Color
// ("gbc"; DMG games run colorized), without a boot ROM.
func newConsole(path, model string) (*gb.GameBoy, error) {
	rom, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cart, err := gb.NewCartridge(rom)
	if err != nil {
		return nil, err
	}
	m := gb.ModelDMG
	if model == "gbc" {
		m = gb.ModelCGB
	}
	return gb.NewModel(cart, nil, m)
}

type held struct {
	button     gb.Button
	start, end int
}

// parsePress reads "start:200-210,right:300-360".
func parsePress(s string) ([]held, error) {
	names := map[string]gb.Button{}
	for _, b := range gb.Buttons {
		names[strings.ToLower(b.String())] = b
	}
	var out []held
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item == "" {
			continue
		}
		name, span, ok := strings.Cut(item, ":")
		b, known := names[strings.ToLower(name)]
		from, to, ok2 := strings.Cut(span, "-")
		start, err1 := strconv.Atoi(from)
		end, err2 := strconv.Atoi(to)
		if !ok || !ok2 || !known || err1 != nil || err2 != nil {
			return nil, fmt.Errorf("bad press %q", item)
		}
		out = append(out, held{b, start, end})
	}
	return out, nil
}

// saveShot runs the ROM for frames frames, holding the pressed buttons, and
// saves the last frame as the screenshot hotkey of the headless mode does.
func saveShot(path, model string, frames int, press, paletteID string, correct bool, compat, scale int, file string) error {
	console, err := newConsole(path, model)
	if err != nil {
		return err
	}
	presses, err := parsePress(press)
	if err != nil {
		return err
	}
	pal := &ui.Palettes[0]
	found := false
	for i := range ui.Palettes {
		if ui.Palettes[i].ID == paletteID {
			pal, found = &ui.Palettes[i], true
		}
	}
	if !found {
		return fmt.Errorf("unknown palette %q", paletteID)
	}
	for f := range frames {
		for _, b := range gb.Buttons {
			down := false
			for _, p := range presses {
				down = down || (p.button == b && f >= p.start && f < p.end)
			}
			console.SetButton(b, down)
		}
		console.SetCompatPalette(compat)
		console.RunFrame()
	}
	out, err := os.Create(file)
	if err != nil {
		return err
	}
	defer out.Close()
	return png.Encode(out, ui.Screenshot(console, pal, correct, scale))
}

// renderFilters draws the PNG src through every display filter.
func renderFilters(src string, colorMode bool, gapHex, size, dir string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return err
	}
	var w, h int
	if _, err := fmt.Sscanf(size, "%dx%d", &w, &h); err != nil {
		return fmt.Errorf("bad size %q", size)
	}
	v, err := strconv.ParseUint(gapHex, 16, 32)
	if err != nil {
		return fmt.Errorf("bad gap color %q", gapHex)
	}
	gap := color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xFF}
	return ui.CaptureFilters(img.(image.Image), colorMode, gap, w, h, dir)
}
