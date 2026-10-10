package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/rom"
)

// tuneSessionFile records 100 frames of a game and writes a session
// comparing two variants on frames 40 to 70 of it.
func tuneSessionFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	game, err := rom.Open(writeROM(t, filepath.Join(dir, "game.gb"), "TUNE", 0), rom.Options{Model: gb.ModelAuto, BootROM: "none"})
	if err != nil {
		t.Fatal(err)
	}
	g := newTestGame(t, newFakePads())
	g.gb, g.title, g.romPath, g.stream = game.Console, game.Title, game.Path, &audioStream{}
	g.recorder = &rumbleRecorder{dir: dir}
	for f := range 100 {
		g.down = [8]bool{4: f >= 50 && f < 60} // A
		g.advance(false, false)
	}
	g.recorder.finish()
	clips, _ := filepath.Glob(filepath.Join(dir, "TUNE-*.json"))
	if len(clips) != 1 {
		t.Fatalf("recordings %v", clips)
	}
	session, _ := json.Marshal(map[string]any{
		"variants": map[string]any{"base": map[string]any{}, "strong": map[string]any{"gain": 3}},
		"pairs":    []map[string]any{{"clip": filepath.Base(clips[0]), "from": 40, "to": 70, "a": "base", "b": "strong"}},
	})
	path := filepath.Join(dir, "session.json")
	if err := os.WriteFile(path, session, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A session plays both versions of a pair, then keeps the answer, with the
// variant chosen and how it shook.
func TestTuner(t *testing.T) {
	path := tuneSessionFile(t)
	tu, err := loadTuner(path)
	if err != nil {
		t.Fatal(err)
	}
	if p := tu.params["strong"]; p.Gain != 3 || p.Floor != gb.DefaultRumbleParams().Floor {
		t.Errorf("variant strong %+v: missing values must be the defaults", p)
	}
	g := newTestGame(t, newFakePads())
	g.stream, g.tune = &audioStream{}, tu
	step := func(a menuActions) {
		g.actions = a
		tu.update(g)
	}
	for range 2 * (tuneGapTicks + 30) { // both versions
		step(menuActions{})
	}
	if tu.step != tuneChoose || g.gb == nil {
		t.Fatalf("after both versions: step %d", tu.step)
	}
	step(menuActions{back: true}) // again
	if tu.step != tuneLoad || tu.version != 0 {
		t.Fatalf("B did not replay the pair: step %d, version %d", tu.step, tu.version)
	}
	for range 2 * (tuneGapTicks + 30) {
		step(menuActions{})
	}
	step(menuActions{right: true}) // the second one
	step(menuActions{left: true})  // shook too little
	if tu.step != tuneDone {
		t.Fatalf("step %d, want done", tu.step)
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(path), "session-answers.json"))
	if err != nil {
		t.Fatal(err)
	}
	var answers []tuneAnswer
	if err := json.Unmarshal(data, &answers); err != nil || len(answers) != 1 {
		t.Fatalf("answers %s: %v", data, err)
	}
	if a := answers[0]; a.Choice != a.Second || a.Hint != "less" || a.First == a.Second || a.From != 40 {
		t.Errorf("answer %+v", a)
	}

	// Started again, the session goes on after the answered pairs.
	if tu, err = loadTuner(path); err != nil || tu.step != tuneDone {
		t.Errorf("reopened: step %d, %v", tu.step, err)
	}
}
