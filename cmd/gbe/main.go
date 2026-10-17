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
	"github.com/ldechoux/gbe/internal/link"
	"github.com/ldechoux/gbe/internal/rom"
	"github.com/ldechoux/gbe/internal/ui"
	"github.com/ldechoux/gbe/internal/ui/scaler"
)

// version is set at build time by the release workflow (-ldflags -X).
var version = "dev"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// runUI opens the window: ui.Run, replaced in tests.
var runUI = ui.Run

// run is gbe with the command-line arguments args: it returns the exit
// status, its errors told on stderr.
func run(args []string, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintln(stderr, "gbe:", err)
		return 1
	}
	flags := flag.NewFlagSet("gbe", flag.ContinueOnError)
	flags.SetOutput(stderr)
	romPath := flags.String("rom", "", "path to the .gb or .gbc ROM, or a .zip archive holding one (may also be given as the first argument; without one, the window waits for a ROM dropped on it)")
	biosPath := flags.String("bios", "", `boot ROM to run first ("none" to skip it; default gb_bios.bin, or gbc_bios.bin in Game Boy Color mode, from a bios folder in the working folder, the config folder or next to gbe; ignored if missing)`)
	modelName := flags.String("model", "auto", `hardware: "auto" (the one the game was made for: Game Boy Color for the games that support it, and for Game Boy games too if colorization is on in the menu), "gb" (or "dmg") or "gbc" (or "cgb"; Game Boy games run colorized)`)
	cfgPath := flags.String("config", "", "config file (default: user config dir/gbe/config.json)")
	scale := flags.Int("scale", 0, "window scale, overrides the config")
	filterName := flags.String("filter", "", "display filter, overrides the config: "+strings.Join(scaler.IDs(), ", "))
	stereo := flags.String("stereo", "", "sound output, overrides the config: "+strings.Join(audiofx.StereoModes, ", ")+" (headphones lets each ear hear a little of the other side)")
	audioFilter := flags.String("audio-filter", "", "audio filter, overrides the config: "+strings.Join(audiofx.Filters, ", "))
	shotDir := flags.String("screenshot-dir", "", "where the screenshot hotkey saves PNGs (default ~/Pictures/gbe)")
	frames := flags.Int("frames", 0, "headless mode: run this many frames without a window, then exit")
	shot := flags.String("screenshot", "", "headless mode: write the last frame to this PNG file")
	inputs := flags.String("input", "", `headless mode: button presses, e.g. "start:200-210,right:300-600"`)
	wav := flags.String("wav", "", "headless mode: record the audio to this WAV file (with -stereo and -audio-filter only, not the config)")
	linkHost := flags.String("link-host", "", `link cable: wait for another gbe to connect on this address, e.g. ":5555" (every interface) or "127.0.0.1:5555"`)
	linkConnect := flags.String("link-connect", "", `link cable: connect to the gbe waiting on this address, e.g. "192.168.1.20:5555"`)
	showVersion := flags.Bool("version", false, "print the version and exit")
	rumbleRecord := flags.String("rumble-record", "", "development: record stretches of play in this folder, to tune the rumble detector (tools/rumblelab, -rumble-tune)")
	rumbleDebug := flags.Bool("rumble-debug", false, "development: show what the rumble detector hears")
	rumbleTune := flags.String("rumble-tune", "", "development: run this tuning session of the rumble detector, comparing its settings on recorded play")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *showVersion {
		fmt.Fprintln(stdout, "gbe", version)
		return 0
	}

	if *romPath == "" && flags.NArg() > 0 {
		*romPath = flags.Arg(0)
	}
	if *romPath == "" && *frames > 0 {
		fmt.Fprintln(stderr, "usage: gbe [flags] game.gb|game.gbc|game.zip")
		flags.PrintDefaults()
		return 2
	}

	if *linkHost != "" && *linkConnect != "" {
		return fail(fmt.Errorf("-link-host and -link-connect: one end each"))
	}
	if (*linkHost != "" || *linkConnect != "") && *frames > 0 {
		return fail(fmt.Errorf("the link cable needs the window, not -frames"))
	}

	model, err := parseModel(*modelName)
	if err != nil {
		return fail(err)
	}
	if _, ok := scaler.ID(*filterName); *filterName != "" && !ok {
		return fail(fmt.Errorf("unknown filter %q (want %s)", *filterName, strings.Join(scaler.IDs(), ", ")))
	}
	stereoID, ok := audiofx.ID(audiofx.StereoModes, *stereo)
	if *stereo != "" && !ok {
		return fail(fmt.Errorf("unknown sound output %q (want %s)", *stereo, strings.Join(audiofx.StereoModes, ", ")))
	}
	audioFilterID, ok := audiofx.ID(audiofx.Filters, *audioFilter)
	if *audioFilter != "" && !ok {
		return fail(fmt.Errorf("unknown audio filter %q (want %s)", *audioFilter, strings.Join(audiofx.Filters, ", ")))
	}
	configPath := *cfgPath
	if configPath == "" {
		configPath = ui.DefaultConfigPath()
	}
	configDir := filepath.Dir(configPath)
	if *frames == 0 {
		logToFile(filepath.Join(configDir, "gbe.log"), stderr)
	}
	cfg, err := ui.LoadConfig(configPath)
	if err != nil {
		log.Printf("config %s: %v (using defaults)", configPath, err)
	}
	bootROMs := &rom.BootROMSearch{Dir: cfg.BootROMDir, Defaults: bootROMDirs(configDir)}
	opts := rom.Options{
		Model:    model,
		BootROM:  *biosPath,
		BootROMs: bootROMs,
		Colorize: cfg.ColorizeDMG,
		Fresh:    *frames > 0,
	}

	// Without a ROM, the window asks for one to be dropped on it. One that
	// cannot be opened is explained there too: launched from the Finder or
	// the Explorer, gbe has no terminal to tell it in.
	var game *rom.Game
	var gameErr error
	if *romPath != "" {
		if game, gameErr = rom.Open(*romPath, opts); gameErr != nil {
			if *frames > 0 {
				return fail(gameErr)
			}
			log.Print(gameErr)
		}
	}

	if *frames > 0 {
		fx := audiofx.New(48000, stereoID, audioFilterID)
		if err := runHeadless(game.Console, *frames, *inputs, *shot, *wav, fx, stdout); err != nil {
			return fail(err)
		}
		return 0
	}

	var cable *link.Cable
	switch {
	case *linkHost != "":
		if cable, err = link.Host(*linkHost, link.LocalName()); err != nil {
			return fail(err)
		}
		log.Printf("link cable: waiting on %v", cable.Addr())
	case *linkConnect != "":
		cable = link.Connect(*linkConnect, link.LocalName())
		log.Printf("link cable: connecting to %s", *linkConnect)
	}
	var end ui.CableEnd
	if cable != nil {
		defer cable.Close()
		end = cable
	}

	if err := runUI(ui.Options{
		Game:        game,
		LaunchPath:  *romPath,
		LaunchError: gameErr,
		BootROMs:    bootROMs,
		ConfigPath:  configPath,
		Scale:       *scale,
		Filter:      *filterName,

		Stereo:      stereoID,
		AudioFilter: audioFilterID,

		ScreenshotDir: *shotDir,
		RumbleRecord:  *rumbleRecord,
		RumbleDebug:   *rumbleDebug,
		RumbleTune:    *rumbleTune,
		Cable:         end,
		// A dropped game opens with the same -model and -bios.
		Open: func(path string, colorize bool) (*rom.Game, error) {
			o := opts
			o.Colorize, o.Strict = colorize, true
			return rom.Open(path, o)
		},
	}); err != nil {
		return fail(err)
	}
	return 0
}

// logToFile copies the log to path, from scratch at each launch: launched
// from the Finder or the Explorer, gbe has no terminal, and the log is the
// only trace of what went wrong.
func logToFile(path string, stderr io.Writer) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("log file: %v", err)
		return
	}
	f, err := os.Create(path)
	if err != nil {
		log.Printf("log file: %v", err)
		return
	}
	log.SetOutput(io.MultiWriter(stderr, f))
}

// bootROMDirs are the folders searched for the default boot ROMs after the
// one chosen in the menu: bios in the working folder, then in the config
// folder, which does not depend on where gbe is launched from, then next to
// the executable.
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

func runHeadless(console *gb.GameBoy, frames int, inputs, shot, wav string, fx *audiofx.Chain, stdout io.Writer) error {
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
		fmt.Fprintf(stdout, "serial: %s\n", console.Serial.Output)
	}
	fmt.Fprintf(stdout, "PC=%04X bootROM=%v\n", console.PC(), console.BootROMActive())
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
