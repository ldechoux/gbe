package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// A recording keeps the state it starts from, the buttons of each frame and
// the marks; a jump in time starts another.
func TestRumbleRecorder(t *testing.T) {
	dir := t.TempDir()
	g := withGameBoy(t, newTestGame(t, newFakePads()), 0x18, 0xFE) // JR -2
	g.stream = &audioStream{}
	g.title, g.romPath = "MENU TEST", "/games/test.gb"
	g.recorder = &rumbleRecorder{dir: dir}
	for f := range 100 {
		g.down = [8]bool{}
		g.down[4] = f >= 10 && f < 20 // A
		g.down[3] = f >= 15           // Right
		if f == 50 {
			g.recorder.mark()
		}
		g.advance(false, false)
	}
	// Back in time: the first stretch is written, and another begins.
	if err := g.gb.LoadState(g.recorder.rec.start); err != nil {
		t.Fatal(err)
	}
	for range 30 {
		g.advance(false, false)
	}
	g.recorder.finish() // under a second: not written

	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(files) != 1 {
		t.Fatalf("recordings %v, want one", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var rec struct {
		ROM, State, Input string
		Frames            int
		Marks             []int
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.ROM != "/games/test.gb" || rec.Frames != 100 || rec.Input != "a:10-20,right:15-100" || len(rec.Marks) != 1 || rec.Marks[0] != 50 {
		t.Errorf("recording %+v", rec)
	}
	if _, err := os.Stat(filepath.Join(dir, rec.State)); err != nil || filepath.Base(rec.State) != rec.State {
		t.Errorf("state %q: %v", rec.State, err)
	}
}
