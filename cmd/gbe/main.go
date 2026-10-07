// Command gbe is a Game Boy (DMG) and Game Boy Color emulator.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ldechoux/gbe/internal/audiofx"
	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/rom"
	"github.com/ldechoux/gbe/internal/ui"
	"github.com/ldechoux/gbe/internal/ui/scaler"
)

// version is set at build time by the release workflow (-ldflags -X).
var version = "dev"

func main() {
	romPath := flag.String("rom", "", "path to the .gb or .gbc ROM, or a .zip archive holding one (may also be given as the first argument; without one, the window waits for a ROM dropped on it)")
	biosPath := flag.String("bios", "", `boot ROM to run first ("none" to skip it; default gb_bios.bin, or gbc_bios.bin in Game Boy Color mode, from a bios folder in the working folder, the config folder or next to gbe; ignored if missing)`)
	modelName := flag.String("model", "auto", `hardware: "auto" (the one the game was made for: Game Boy Color for the games that support it, and for Game Boy games too if colorization is on in the menu), "gb" (or "dmg") or "gbc" (or "cgb"; Game Boy games run colorized)`)
	cfgPath := flag.String("config", "", "config file (default: user config dir/gbe/config.json)")
	scale := flag.Int("scale", 0, "window scale, overrides the config")
	filterName := flag.String("filter", "", "display filter, overrides the config: "+strings.Join(scaler.IDs(), ", "))
	stereo := flag.String("stereo", "", "sound output, overrides the config: "+strings.Join(audiofx.StereoModes, ", ")+" (headphones lets each ear hear a little of the other side)")
	audioFilter := flag.String("audio-filter", "", "audio filter, overrides the config: "+strings.Join(audiofx.Filters, ", "))
	shotDir := flag.String("screenshot-dir", "", "where the screenshot hotkey saves PNGs (default ~/Pictures/gbe)")
	frames := flag.Int("frames", 0, "headless mode: run this many frames without a window, then exit")
	shot := flag.String("screenshot", "", "headless mode: write the last frame to this PNG file")
	inputs := flag.String("input", "", `headless mode: button presses, e.g. "start:200-210,right:300-600"`)
	wav := flag.String("wav", "", "headless mode: record the audio to this WAV file (with -stereo and -audio-filter only, not the config)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("gbe", version)
		return
	}

	if *romPath == "" && flag.NArg() > 0 {
		*romPath = flag.Arg(0)
	}
	if *romPath == "" && *frames > 0 {
		fmt.Fprintln(os.Stderr, "usage: gbe [flags] game.gb|game.gbc|game.zip")
		flag.PrintDefaults()
		os.Exit(2)
	}

	model, err := parseModel(*modelName)
	if err != nil {
		log.Fatal(err)
	}
	if _, ok := scaler.ID(*filterName); *filterName != "" && !ok {
		log.Fatalf("unknown filter %q (want %s)", *filterName, strings.Join(scaler.IDs(), ", "))
	}
	stereoID, ok := audiofx.ID(audiofx.StereoModes, *stereo)
	if *stereo != "" && !ok {
		log.Fatalf("unknown sound output %q (want %s)", *stereo, strings.Join(audiofx.StereoModes, ", "))
	}
	audioFilterID, ok := audiofx.ID(audiofx.Filters, *audioFilter)
	if *audioFilter != "" && !ok {
		log.Fatalf("unknown audio filter %q (want %s)", *audioFilter, strings.Join(audiofx.Filters, ", "))
	}
	configPath := *cfgPath
	if configPath == "" {
		configPath = ui.DefaultConfigPath()
	}
	configDir := filepath.Dir(configPath)
	if *frames == 0 {
		logToFile(filepath.Join(configDir, "gbe.log"))
	}
	cfg, err := ui.LoadConfig(configPath)
	if err != nil {
		log.Printf("config %s: %v (using defaults)", configPath, err)
	}
	opts := rom.Options{
		Model:       model,
		BootROM:     *biosPath,
		BootROMDirs: bootROMDirs(configDir),
		Colorize:    cfg.ColorizeDMG,
		Fresh:       *frames > 0,
	}

	// Without a ROM, the window asks for one to be dropped on it. One that
	// cannot be opened is explained there too: launched from the Finder or
	// the Explorer, gbe has no terminal to tell it in.
	var game *rom.Game
	var gameErr error
	if *romPath != "" {
		if game, gameErr = rom.Open(*romPath, opts); gameErr != nil {
			if *frames > 0 {
				log.Fatal(gameErr)
			}
			log.Print(gameErr)
		}
	}

	if *frames > 0 {
		fx := audiofx.New(48000, stereoID, audioFilterID)
		if err := runHeadless(game.Console, *frames, *inputs, *shot, *wav, fx); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := ui.Run(ui.Options{
		Game:        game,
		LaunchPath:  *romPath,
		LaunchError: gameErr,
		ConfigPath:  configPath,
		Scale:       *scale,
		Filter:      *filterName,

		Stereo:      stereoID,
		AudioFilter: audioFilterID,

		ScreenshotDir: *shotDir,
		// A dropped game opens with the same -model and -bios.
		Open: func(path string, colorize bool) (*rom.Game, error) {
			o := opts
			o.Colorize, o.Strict = colorize, true
			return rom.Open(path, o)
		},
	}); err != nil {
		log.Fatal(err)
	}
}

// logToFile copies the log to path, from scratch at each launch: launched
// from the Finder or the Explorer, gbe has no terminal, and the log is the
// only trace of what went wrong.
func logToFile(path string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("log file: %v", err)
		return
	}
	f, err := os.Create(path)
	if err != nil {
		log.Printf("log file: %v", err)
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
}

// bootROMDirs are the folders searched for the default boot ROMs: bios in
// the working folder, then in the config folder, which does not depend on
// where gbe is launched from, then next to the executable.
func bootROMDirs(configDir string) []string {
	dirs := []string{"bios", filepath.Join(configDir, "bios")}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Join(filepath.Dir(exe), "bios"))
	}
	return dirs
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

func runHeadless(console *gb.GameBoy, frames int, inputs, shot, wav string, fx *audiofx.Chain) error {
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
		samples := console.APU.DrainSamples()
		fx.Process(samples)
		pcm = append(pcm, samples...)
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
