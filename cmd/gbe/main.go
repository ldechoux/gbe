// Command gbe is a Game Boy (DMG) emulator.
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
	"strconv"
	"strings"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/ui"
)

// version is set at build time by the release workflow (-ldflags -X).
var version = "dev"

func main() {
	romPath := flag.String("rom", "", "path to the .gb ROM (may also be given as the first argument)")
	biosPath := flag.String("bios", "bios/gb_bios.bin", `boot ROM to run first ("none" to skip it; ignored if missing)`)
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
		fmt.Fprintln(os.Stderr, "usage: gbe [flags] game.gb")
		flag.PrintDefaults()
		os.Exit(2)
	}

	rom, err := os.ReadFile(*romPath)
	if err != nil {
		log.Fatal(err)
	}
	cart, err := gb.NewCartridge(rom)
	if err != nil {
		log.Fatal(err)
	}
	savePath := strings.TrimSuffix(*romPath, ".gb") + ".sav"
	if cart.Battery {
		if data, err := os.ReadFile(savePath); err == nil {
			cart.LoadSaveData(data)
		} else if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("reading save: %v", err)
		}
	}

	var boot []byte
	if *biosPath != "none" && *biosPath != "" {
		boot, err = os.ReadFile(*biosPath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Fatal(err)
		}
	}
	console, err := gb.New(cart, boot)
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
		StatePath:  strings.TrimSuffix(*romPath, ".gb") + ".state",
		ConfigPath: *cfgPath,
		Scale:      *scale,

		ScreenshotDir: *shotDir,
	}); err != nil {
		log.Fatal(err)
	}
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
	img := ui.ScreenshotImage(console.Framebuffer(), &ui.Palettes[0], 1)
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
