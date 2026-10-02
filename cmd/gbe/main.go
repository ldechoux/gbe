// Command gbe is a Game Boy (DMG) and Game Boy Color emulator.
package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/ui"
)

// version is set at build time by the release workflow (-ldflags -X).
var version = "dev"

func main() {
	romPath := flag.String("rom", "", "path to the .gb or .gbc ROM, or a .zip archive holding one (may also be given as the first argument)")
	biosPath := flag.String("bios", "", `boot ROM to run first ("none" to skip it; default bios/gb_bios.bin, or bios/gbc_bios.bin in Game Boy Color mode; ignored if missing)`)
	modelName := flag.String("model", "auto", `hardware: "auto" (the one the game was made for: Game Boy Color for the games that support it, and for Game Boy games too if colorization is on in the menu), "gb" (or "dmg") or "gbc" (or "cgb"; Game Boy games run colorized)`)
	cfgPath := flag.String("config", "", "config file (default: user config dir/gbe/config.json)")
	scale := flag.Int("scale", 0, "window scale, overrides the config")
	shotDir := flag.String("screenshot-dir", "", "where the screenshot hotkey saves PNGs (default ~/Pictures/gbe)")
	frames := flag.Int("frames", 0, "headless mode: run this many frames without a window, then exit")
	shot := flag.String("screenshot", "", "headless mode: write the last frame to this PNG file")
	inputs := flag.String("input", "", `headless mode: button presses, e.g. "start:200-210,right:300-600"`)
	wav := flag.String("wav", "", "headless mode: record the audio to this WAV file")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("gbe", version)
		return
	}

	if *romPath == "" && flag.NArg() > 0 {
		*romPath = flag.Arg(0)
	}
	if *romPath == "" {
		fmt.Fprintln(os.Stderr, "usage: gbe [flags] game.gb|game.gbc|game.zip")
		flag.PrintDefaults()
		os.Exit(2)
	}

	rom, entry, err := readROM(*romPath)
	if err != nil {
		log.Fatal(err)
	}
	if entry != "" {
		log.Printf("%s: %s", *romPath, entry)
	}
	cart, err := gb.NewCartridge(rom)
	if err != nil {
		log.Fatal(err)
	}
	model, err := parseModel(*modelName)
	if err != nil {
		log.Fatal(err)
	}
	configPath := *cfgPath
	if configPath == "" {
		configPath = ui.DefaultConfigPath()
	}
	cfg, err := ui.LoadConfig(configPath)
	if err != nil {
		log.Printf("config %s: %v (using defaults)", configPath, err)
	}
	preferred := gb.ResolveModel(cart, model)
	if model == gb.ModelAuto && !cart.ColorSupported() && cfg.ColorizeDMG {
		preferred = gb.ModelCGB // a Game Boy Color colorizes DMG games (opt-in)
	}
	base := romBase(*romPath)
	savePath, statePath := base+".sav", base+".state"
	if cart.Battery {
		if data, err := os.ReadFile(savePath); err == nil {
			cart.LoadSaveData(data)
		} else if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("reading save: %v", err)
		}
	}

	newConsole := func(model gb.Model) (*gb.GameBoy, error) {
		path := *biosPath
		if path == "" {
			path = "bios/gb_bios.bin"
			if model == gb.ModelCGB {
				path = "bios/gbc_bios.bin"
			}
		}
		var boot []byte
		if path != "none" {
			var err error
			if boot, err = os.ReadFile(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
		}
		return gb.NewModel(cart, boot, model)
	}

	// Left to auto, the console starts in the hardware mode of the save
	// state, which could not be resumed otherwise (e.g. after colorizing DMG
	// games). The menu goes to the preferred one when the player starts over
	// or resets.
	launch := preferred
	if data, err := os.ReadFile(statePath); err == nil && model == gb.ModelAuto && *frames == 0 {
		if m, err := gb.StateModel(data); err == nil && (m == gb.ModelDMG || m == gb.ModelCGB) {
			launch = m
		}
	}
	console, err := newConsole(launch)
	if err != nil {
		log.Fatal(err)
	}

	if *frames > 0 {
		if err := runHeadless(console, *frames, *inputs, *shot, *wav); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := ui.Run(ui.Options{
		GameBoy:    console,
		Title:      cart.Title,
		SavePath:   savePath,
		StatePath:  statePath,
		ConfigPath: configPath,
		Scale:      *scale,

		ScreenshotDir: *shotDir,
		AutoModel:     model == gb.ModelAuto,
		NewConsole:    newConsole,
	}); err != nil {
		log.Fatal(err)
	}
}

// parseModel reads the -model flag. "gb" and "gbc" are the names players
// know; "dmg" and "cgb", the hardware codes, are accepted too.
func parseModel(name string) (gb.Model, error) {
	switch strings.ToLower(name) {
	case "auto":
		return gb.ModelAuto, nil
	case "gb", "dmg":
		return gb.ModelDMG, nil
	case "gbc", "cgb":
		return gb.ModelCGB, nil
	}
	return 0, fmt.Errorf("unknown model %q (want auto, gb or gbc)", name)
}

// romBase is the ROM path without its .gb, .gbc or .zip extension, to which
// the save files extensions are appended: the saves of a zipped ROM sit next
// to the archive, which is never written.
func romBase(path string) string {
	ext := filepath.Ext(path)
	if isROMName(path) || strings.EqualFold(ext, ".zip") {
		return strings.TrimSuffix(path, ext)
	}
	return path
}

type press struct {
	button     gb.Button
	start, end int
}

func parseInputs(s string) ([]press, error) {
	names := map[string]gb.Button{}
	for _, b := range gb.Buttons {
		names[strings.ToLower(b.String())] = b
	}
	var out []press
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item == "" {
			continue
		}
		name, span, ok := strings.Cut(item, ":")
		b, known := names[strings.ToLower(name)]
		from, to, ok2 := strings.Cut(span, "-")
		if !ok || !ok2 || !known {
			return nil, fmt.Errorf("bad input %q", item)
		}
		start, err1 := strconv.Atoi(from)
		end, err2 := strconv.Atoi(to)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("bad input %q", item)
		}
		out = append(out, press{b, start, end})
	}
	return out, nil
}

func runHeadless(console *gb.GameBoy, frames int, inputs, shot, wav string) error {
	presses, err := parseInputs(inputs)
	if err != nil {
		return err
	}
	var pcm []int16
	for f := range frames {
		for _, b := range gb.Buttons {
			down := false
			for _, p := range presses {
				if p.button == b && f >= p.start && f < p.end {
					down = true
				}
			}
			console.SetButton(b, down)
		}
		console.RunFrame()
		pcm = append(pcm, console.APU.DrainSamples()...)
	}
	if wav != "" {
		if err := writeWAV(wav, pcm, 48000); err != nil {
			return err
		}
	}
	if len(console.Serial.Output) > 0 {
		fmt.Printf("serial: %s\n", console.Serial.Output)
	}
	fmt.Printf("PC=%04X bootROM=%v\n", console.PC(), console.BootROMActive())
	if shot == "" {
		return nil
	}
	img := ui.Screenshot(console, &ui.Palettes[0], false, 1)
	f, err := os.Create(shot)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// writeWAV stores interleaved stereo 16-bit samples as a PCM WAV file.
func writeWAV(path string, pcm []int16, rate int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	size := uint32(len(pcm) * 2)
	hdr := []any{
		[4]byte{'R', 'I', 'F', 'F'}, 36 + size, [4]byte{'W', 'A', 'V', 'E'},
		[4]byte{'f', 'm', 't', ' '}, uint32(16), uint16(1), uint16(2),
		uint32(rate), uint32(rate * 4), uint16(4), uint16(16),
		[4]byte{'d', 'a', 't', 'a'}, size,
	}
	for _, v := range hdr {
		if err := binary.Write(f, binary.LittleEndian, v); err != nil {
			return err
		}
	}
	return binary.Write(f, binary.LittleEndian, pcm)
}
