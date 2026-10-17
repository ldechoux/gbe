package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/link"
	"github.com/ldechoux/gbe/internal/ui"
)

func TestParseModel(t *testing.T) {
	for name, want := range map[string]gb.Model{
		"auto": gb.ModelAuto,
		"gb":   gb.ModelDMG, "dmg": gb.ModelDMG,
		"gbc": gb.ModelCGB, "cgb": gb.ModelCGB, "GBC": gb.ModelCGB,
	} {
		if got, err := parseModel(name); err != nil || got != want {
			t.Errorf("%q: %v %v, want %v", name, got, err, want)
		}
	}
	if _, err := parseModel("gba"); err == nil {
		t.Error("unknown model accepted")
	}
}

const benchROM = "../../benchrom/gbe-bench.gbc"

// gbe runs the command with args, and returns its exit status and output.
func gbe(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	defer log.SetOutput(log.Writer())
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRunVersion(t *testing.T) {
	if code, out, _ := gbe(t, "-version"); code != 0 || out != "gbe dev\n" {
		t.Errorf("-version: %d %q", code, out)
	}
}

// The headless mode runs a game, holding buttons, and saves its last frame
// and its sound.
func TestRunHeadless(t *testing.T) {
	dir := t.TempDir()
	shot, wav := filepath.Join(dir, "out.png"), filepath.Join(dir, "out.wav")
	code, out, errs := gbe(t, "-config", filepath.Join(dir, "c.json"), "-bios", "none", "-frames", "120",
		"-input", "a:0-10", "-screenshot", shot, "-wav", wav, "-stereo", "headphones", "-audio-filter", "soft", benchROM)
	if code != 0 || !strings.HasPrefix(out, "PC=") || !strings.Contains(out, "bootROM=false") {
		t.Fatalf("exit %d, output %q, errors %q", code, out, errs)
	}
	f, err := os.Open(shot)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil || img.Bounds().Dx() != gb.ScreenWidth || img.Bounds().Dy() != gb.ScreenHeight {
		t.Errorf("screenshot: %v, %v", err, img.Bounds())
	}
	data, err := os.ReadFile(wav)
	if err != nil {
		t.Fatal(err)
	}
	// 120 frames at 48 kHz: about 2 s of stereo 16-bit samples.
	size := binary.LittleEndian.Uint32(data[40:44])
	if string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" || int(size) != len(data)-44 || size < 360000 || size > 400000 {
		t.Errorf("WAV: %q, %d bytes of samples, file of %d", data[:12], size, len(data))
	}
	if _, err := os.Stat(filepath.Join(dir, "gbe.log")); err == nil {
		t.Error("the headless mode must not write gbe.log")
	}
}

// Wrong arguments are refused before anything runs.
func TestRunErrors(t *testing.T) {
	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"-frames", "10"}, 2, "usage: gbe"},
		{[]string{"-nope"}, 2, "flag provided but not defined"},
		{[]string{"-model", "gba", benchROM}, 1, `unknown model "gba"`},
		{[]string{"-filter", "blur", benchROM}, 1, `unknown filter "blur"`},
		{[]string{"-stereo", "5.1", benchROM}, 1, `unknown sound output "5.1"`},
		{[]string{"-audio-filter", "loud", benchROM}, 1, `unknown audio filter "loud"`},
		{[]string{"-frames", "10", "missing.gb"}, 1, "missing.gb"},
		{[]string{"-frames", "10", "-bios", "none", "-input", "jump:1-2", benchROM}, 1, `bad input "jump:1-2"`},
	} {
		args := append([]string{"-config", filepath.Join(t.TempDir(), "c.json")}, c.args...)
		code, _, errs := gbe(t, args...)
		if code != c.code || !strings.Contains(errs, c.want) {
			t.Errorf("%v: exit %d, %q; want %d, %q", c.args, code, errs, c.code, c.want)
		}
	}
}

// Without -frames, the window opens with the game and the settings of the
// command line; a game that cannot be opened is told in it.
func TestRunWindow(t *testing.T) {
	defer func(f func(ui.Options) error) { runUI = f }(runUI)
	var got ui.Options
	runUI = func(o ui.Options) error { got = o; return nil }
	dir := t.TempDir()
	cfg := filepath.Join(dir, "c.json")
	code, _, errs := gbe(t, "-config", cfg, "-scale", "3", "-filter", "lcd", "-stereo", "headphones", "-bios", "none", benchROM)
	if code != 0 || got.Game == nil || got.Scale != 3 || got.Filter != "lcd" || got.Stereo != "headphones" || got.ConfigPath != cfg {
		t.Fatalf("exit %d (%q), options %+v", code, errs, got)
	}
	if _, err := os.Stat(filepath.Join(dir, "gbe.log")); err != nil {
		t.Errorf("gbe.log: %v", err)
	}
	// A game dropped later opens with the same -bios, colorized or not.
	if next, err := got.Open(benchROM, true); err != nil || next.Console.BootROMActive() {
		t.Errorf("dropped game: %v", err)
	}

	code, _, _ = gbe(t, "-config", cfg, filepath.Join(dir, "missing.gb"))
	if code != 0 || got.Game != nil || got.LaunchError == nil || got.LaunchPath == "" {
		t.Errorf("missing game: exit %d, options %+v", code, got)
	}

	runUI = func(ui.Options) error { return errors.New("no display") }
	if code, _, errs := gbe(t, "-config", cfg); code != 1 || !strings.Contains(errs, "no display") {
		t.Errorf("window error: exit %d, %q", code, errs)
	}
}

func TestParseInputs(t *testing.T) {
	got, err := parseInputs(" Start:200-210, right:300-600 ,")
	if err != nil || len(got) != 2 || got[0] != (press{gb.ButtonStart, 200, 210}) || got[1] != (press{gb.ButtonRight, 300, 600}) {
		t.Errorf("%+v, %v", got, err)
	}
	for _, bad := range []string{"start", "start:200", "start:a-b", "turbo:1-2"} {
		if _, err := parseInputs(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// The boot ROMs are searched in the working folder, then the config one,
// then next to the executable.
func TestBootROMDirs(t *testing.T) {
	dirs := bootROMDirs("/config/gbe")
	if len(dirs) != 3 || dirs[0] != "bios" || dirs[1] != filepath.Join("/config/gbe", "bios") || filepath.Base(dirs[2]) != "bios" {
		t.Errorf("%v", dirs)
	}
}

func TestWriteWAVError(t *testing.T) {
	if err := writeWAV(filepath.Join(t.TempDir(), "missing", "a.wav"), nil, 48000); err == nil {
		t.Error("written in a missing folder")
	}
}

// -link-host and -link-connect hand the window each end of a link cable.
func TestRunLinkCable(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "c.json")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"-link-host", ":0", "-link-connect", "127.0.0.1:1"}, "one end each"},
		{[]string{"-link-host", ":0", "-frames", "1", benchROM}, "needs the window"},
		{[]string{"-link-host", "256.0.0.1:0"}, "listen"},
	} {
		if code, _, errs := gbe(t, append([]string{"-config", cfg}, c.args...)...); code != 1 || !strings.Contains(errs, c.want) {
			t.Errorf("%v: exit %d, %q; want %q", c.args, code, errs, c.want)
		}
	}

	defer func(f func(ui.Options) error) { runUI = f }(runUI)
	var host *link.Cable
	runUI = func(o ui.Options) error {
		host = o.Cable.(*link.Cable)
		// The other end, launched while this one waits.
		runUI = func(o ui.Options) error {
			for _, c := range []ui.CableEnd{host, o.Cable} {
				select {
				case <-c.Links():
				case <-time.After(5 * time.Second):
					return errors.New("not connected")
				}
			}
			return nil
		}
		if code, _, errs := gbe(t, "-config", cfg, "-link-connect", host.Addr().String()); code != 0 {
			return fmt.Errorf("-link-connect: exit %d, %q", code, errs)
		}
		return nil
	}
	if code, _, errs := gbe(t, "-config", cfg, "-link-host", "127.0.0.1:0"); code != 0 {
		t.Fatalf("-link-host: exit %d, %q", code, errs)
	}
}

// Each gbe running at once writes its own log.
func TestLogPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "locks"), 0o755); err != nil {
		t.Fatal(err)
	}
	defer func(f *os.File) { logLock = f }(logLock)
	first := logPath(dir)
	held := logLock
	second := logPath(dir)
	defer held.Close()
	defer logLock.Close()
	if first != filepath.Join(dir, "gbe.log") || second != filepath.Join(dir, "gbe-2.log") {
		t.Errorf("logs %s and %s", first, second)
	}
}
