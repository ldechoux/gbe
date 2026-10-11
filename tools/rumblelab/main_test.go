package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ldechoux/gbe/internal/gb"
)

func TestButtons(t *testing.T) {
	ps, err := parseButtons("right:180/240,a:12/40")
	if err != nil {
		t.Fatal(err)
	}
	s := scripted(ps)
	// gb.Buttons: up, down, left, right, a, b, start, select.
	if d := s(0); !d[3] || !d[4] {
		t.Errorf("frame 0: %v", d)
	}
	if d := s(215); d[3] || d[4] {
		t.Errorf("frame 215: %v", d)
	}
	r, err := replayed("a:10-20,right:15-100")
	if err != nil {
		t.Fatal(err)
	}
	for f, want := range map[int][8]bool{9: {}, 10: {4: true}, 15: {3: true, 4: true}, 20: {3: true}, 100: {}} {
		if got := r(f); got != want {
			t.Errorf("frame %d: %v, want %v", f, got, want)
		}
	}
	if _, err := replayed("jump:1-2"); err == nil {
		t.Error("an unknown button is accepted")
	}
}

const benchROM = "../../benchrom/gbe-bench.gbc"

// The page of a run: its notes, the vibration, the sound and the screens.
func TestRunPage(t *testing.T) {
	out := t.TempDir()
	var summary bytes.Buffer
	if err := runLab([]string{"-frames", "120", "-buttons", "a:10/60", "-bios", t.TempDir(), "-out", out, benchROM}, &summary); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.html", "data.json", "sound.wav", "screens.jpg"} {
		if fi, err := os.Stat(filepath.Join(out, name)); err != nil || fi.Size() == 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
	if summary.Len() == 0 {
		t.Error("no summary")
	}
}

// A recorded stretch of play replays from its state, with its buttons;
// -quick only writes the data, with the settings of a file.
func TestRunRecordingQuick(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(benchROM)
	if err != nil {
		t.Fatal(err)
	}
	cart, err := gb.NewCartridge(data)
	if err != nil {
		t.Fatal(err)
	}
	g, err := gb.NewModel(cart, nil, gb.ModelCGB) // the hardware rumblelab picks for this ROM
	if err != nil {
		t.Fatal(err)
	}
	g.RunFrame()
	write := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write("clip.state", g.SaveState())
	abs, _ := filepath.Abs(benchROM)
	rec, _ := json.Marshal(map[string]any{"rom": abs, "state": "clip.state", "frames": 90, "input": "a:0-10,right:20-80"})
	clip := write("clip.json", rec)
	params := write("params.json", []byte(`{"gain": 2}`))
	out := filepath.Join(dir, "out")
	var summary bytes.Buffer
	if err := runLab([]string{"-quick", "-params", params, "-bios", dir, "-out", out, clip}, &summary); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	data, err = os.ReadFile(filepath.Join(out, "data.json"))
	if err != nil || json.Unmarshal(data, &got) != nil || got["level"] == nil {
		t.Errorf("data.json: %v %s", err, data)
	}
	if _, err := os.Stat(filepath.Join(out, "index.html")); err == nil {
		t.Error("-quick wrote the page")
	}
}

func TestRunErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{}, flag.ErrHelp.Error()},
		{[]string{"-buttons", "jump:1/2", benchROM}, "jump"},
		{[]string{"-params", filepath.Join(dir, "missing.json"), benchROM}, "missing.json"},
		{[]string{"-params", bad, benchROM}, "unexpected end"},
		{[]string{bad}, "bad.json"},
		{[]string{filepath.Join(dir, "missing.json")}, "missing.json"},
		{[]string{"-frames", "1", filepath.Join(dir, "missing.gb")}, "missing.gb"},
	} {
		err := runLab(append([]string{"-out", dir}, c.args...), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), c.want) && !errors.Is(err, flag.ErrHelp) {
			t.Errorf("%v: %v, want %q", c.args, err, c.want)
		}
	}
}
