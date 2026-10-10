package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/rom"
)

// A tuning session (-rumble-tune) compares settings of the rumble detector
// by feel: it replays stretches of recorded play (see rumbleRecorder) twice,
// with two settings, the gamepad shaking as in a game; the player only
// watches, then tells which shook better, and whether it shook too much or
// not enough. The answers go to a file next to the session, to choose the
// settings of the next one.

// tuneSession is the file a session is read from.
type tuneSession struct {
	// Variants are the settings compared, by name. Missing values are
	// those of gb.DefaultRumbleParams.
	Variants map[string]json.RawMessage `json:"variants"`
	Pairs    []tunePair                 `json:"pairs"`
}

// tunePair compares two variants on a stretch of a recording: frames From
// to To of the recording Clip (a path relative to the session).
type tunePair struct {
	Clip     string `json:"clip"`
	From, To int
	A, B     string
}

// tuneAnswer is what the player answered on a pair.
type tuneAnswer struct {
	Pair   int    `json:"pair"`
	Clip   string `json:"clip"`
	From   int    `json:"from"`
	To     int    `json:"to"`
	First  string `json:"first"`  // the variant shown first
	Second string `json:"second"` // and second
	Choice string `json:"choice"` // the variant preferred, or "same"
	Hint   string `json:"hint"`   // the preferred one shook "less" than it should, "ok", or "more"
	When   string `json:"when"`
}

// Steps of a pair.
const (
	tuneLoad   = iota // the version to play is made ready
	tunePlay          // it plays
	tuneChoose        // which was better?
	tuneHint          // did it shake too much?
	tuneDone          // no pair left
)

// tuneGapTicks is the pause before each version, its number shown.
const tuneGapTicks = 60

type tuner struct {
	dir      string
	path     string // of the answers
	params   map[string]gb.RumbleParams
	pairs    []tunePair
	answers  []tuneAnswer
	pair     int
	order    [2]string // the variants of the pair, in the order shown
	version  int       // 0 or 1, shown or showing
	step     int
	gap      int        // ticks left before the version plays
	frame    int        // of the recording, while playing
	buttons  []tuneSpan // of the recording
	bootROMs *rom.BootROMSearch
	// ready is the console of the pair, at the start of its stretch as
	// snapshot, the detector having learned memory.
	ready    *gb.GameBoy
	title    string
	snapshot []byte
	memory   gb.RumbleMemory
	choice   string
	recorded map[string]*recordedPlay
}

// recordedPlay is a stretch of play as rumbleRecorder writes it.
type recordedPlay struct {
	ROM    string `json:"rom"`
	State  string `json:"state"`
	Frames int    `json:"frames"`
	Input  string `json:"input"`
	dir    string
}

// tuneSpan is a button held from a frame to before another.
type tuneSpan struct{ button, from, to int }

// loadTuner reads the session at path, and the answers already given, so
// that a session stopped halfway goes on.
func loadTuner(path string) (*tuner, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s tuneSession
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	t := &tuner{
		dir: filepath.Dir(path), path: strings.TrimSuffix(path, filepath.Ext(path)) + "-answers.json",
		params: map[string]gb.RumbleParams{}, pairs: s.Pairs, recorded: map[string]*recordedPlay{},
	}
	for name, raw := range s.Variants {
		p := gb.DefaultRumbleParams()
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("variant %s: %w", name, err)
		}
		t.params[name] = p
	}
	for i, p := range t.pairs {
		_, okA := t.params[p.A]
		_, okB := t.params[p.B]
		if !okA || !okB || p.To <= p.From {
			return nil, fmt.Errorf("pair %d: unknown variant or empty stretch", i+1)
		}
	}
	if data, err := os.ReadFile(t.path); err == nil {
		if err := json.Unmarshal(data, &t.answers); err != nil {
			return nil, fmt.Errorf("%s: %w", t.path, err)
		}
		t.pair = len(t.answers)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	t.start()
	return t, nil
}

// start begins the current pair, in a random order: the player cannot tell
// which variant plays first.
func (t *tuner) start() {
	if t.pair >= len(t.pairs) {
		t.step = tuneDone
		return
	}
	p := t.pairs[t.pair]
	t.order = [2]string{p.A, p.B}
	if rand.IntN(2) == 1 {
		t.order = [2]string{p.B, p.A}
	}
	t.version, t.ready = 0, nil
	t.step, t.gap = tuneLoad, tuneGapTicks
}

// replay plays both versions of the pair again.
func (t *tuner) replay() {
	t.version = 0
	t.step, t.gap = tuneLoad, tuneGapTicks
}

// update runs one tick of the session.
func (t *tuner) update(g *Game) {
	a := g.actions
	switch t.step {
	case tuneLoad:
		g.rumble.stop(g.pads)
		if t.gap--; t.gap > 0 {
			return
		}
		if err := t.load(g); err != nil {
			log.Printf("rumble tuning: %v", err)
			g.notifyLong(err.Error())
			t.answer(g, "skipped", "")
			return
		}
		t.step = tunePlay
	case tunePlay:
		if a.back {
			t.replay()
			return
		}
		t.press(g.gb, t.frame)
		g.advance(false, false)
		t.frame++
		if t.frame >= t.pairs[t.pair].To {
			g.rumble.stop(g.pads)
			if t.version == 0 {
				t.version, t.step, t.gap = 1, tuneLoad, tuneGapTicks
			} else {
				t.step = tuneChoose
			}
		}
	case tuneChoose:
		switch {
		case a.back:
			t.replay()
		case a.left:
			t.choice, t.step = t.order[0], tuneHint
		case a.right:
			t.choice, t.step = t.order[1], tuneHint
		case a.down:
			t.answer(g, "same", "")
		}
	case tuneHint:
		switch {
		case a.back:
			t.step = tuneChoose
		case a.left:
			t.answer(g, t.choice, "less")
		case a.ok:
			t.answer(g, t.choice, "ok")
		case a.right:
			t.answer(g, t.choice, "more")
		}
	}
}

// load makes the console of the version to play ready: the recording, run
// to the start of the stretch with the detector listening, as in the game.
// It runs once per pair: both versions start from the same snapshot, the
// detector having learned the same.
func (t *tuner) load(g *Game) error {
	if t.ready == nil {
		if err := t.prepare(); err != nil {
			return err
		}
	}
	console := t.ready
	if err := console.Restore(t.snapshot); err != nil {
		return err
	}
	console.SetRumbleMemory(t.memory)
	console.SetRumbleParams(t.params[t.order[t.version]])
	t.frame = t.pairs[t.pair].From
	g.gb, g.title = console, t.title
	g.rewind.clear()
	if g.stream != nil {
		g.stream.reset()
	}
	g.keepPrevious()
	return nil
}

// prepare runs the recording of the pair to the start of its stretch.
func (t *tuner) prepare() error {
	p := t.pairs[t.pair]
	rec, err := t.recording(p.Clip)
	if err != nil {
		return err
	}
	game, err := rom.Open(rec.ROM, rom.Options{Model: gb.ModelAuto, BootROMs: t.bootROMs, Fresh: true})
	if err != nil {
		return err
	}
	state, err := os.ReadFile(filepath.Join(rec.dir, rec.State))
	if err != nil {
		return err
	}
	console := game.Console
	if err := console.LoadState(state); err != nil {
		return err
	}
	t.buttons = spans(rec.Input)
	console.GuessRumble(true)
	for f := 0; f < p.From; f++ {
		t.press(console, f)
		console.RunFrame()
		console.APU.DrainSamples()
	}
	t.ready, t.title = console, game.Title
	t.snapshot, t.memory = console.Snapshot(nil), console.RumbleMemory()
	return nil
}

// press holds the buttons of the recording on frame f.
func (t *tuner) press(console *gb.GameBoy, f int) {
	for i, b := range gb.Buttons {
		down := false
		for _, s := range t.buttons {
			if s.button == i && f >= s.from && f < s.to {
				down = true
			}
		}
		console.SetButton(b, down)
	}
}

// recording reads the recording at path, relative to the session.
func (t *tuner) recording(path string) (*recordedPlay, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.dir, path)
	}
	if r := t.recorded[path]; r != nil {
		return r, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r := &recordedPlay{dir: filepath.Dir(path)}
	if err := json.Unmarshal(data, r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	t.recorded[path] = r
	return r, nil
}

// spans reads the buttons of a recording: "a:10-25,right:0-300".
func spans(input string) []tuneSpan {
	var out []tuneSpan
	for _, part := range strings.Split(input, ",") {
		name, rest, ok1 := strings.Cut(part, ":")
		from, to, ok2 := strings.Cut(rest, "-")
		f, err1 := strconv.Atoi(from)
		e, err2 := strconv.Atoi(to)
		if !ok1 || !ok2 || err1 != nil || err2 != nil {
			continue
		}
		for i, b := range gb.Buttons {
			if strings.EqualFold(b.String(), name) {
				out = append(out, tuneSpan{i, f, e})
			}
		}
	}
	return out
}

// answer keeps the answer on the current pair, writes the answers, and goes
// to the next pair.
func (t *tuner) answer(g *Game, choice, hint string) {
	p := t.pairs[t.pair]
	t.answers = append(t.answers, tuneAnswer{
		Pair: t.pair + 1, Clip: p.Clip, From: p.From, To: p.To,
		First: t.order[0], Second: t.order[1], Choice: choice, Hint: hint,
		When: time.Now().Format(time.RFC3339),
	})
	data, err := json.MarshalIndent(t.answers, "", "  ")
	if err == nil {
		err = os.WriteFile(t.path, data, 0o644)
	}
	if err != nil {
		log.Printf("rumble tuning: %v", err)
	}
	t.pair++
	t.start()
	if t.step == tuneDone {
		g.rumble.stop(g.pads)
		log.Printf("rumble tuning: answers in %s", t.path)
	}
}

// text is what the screen shows: the pair, the version, the question.
func (t *tuner) text() string {
	n := fmt.Sprintf("Paire %d/%d", min(t.pair+1, len(t.pairs)), len(t.pairs))
	switch t.step {
	case tuneLoad:
		return fmt.Sprintf("%s - version %d", n, t.version+1)
	case tunePlay:
		return fmt.Sprintf("%s - version %d   (B: revoir)", n, t.version+1)
	case tuneChoose:
		return n + " - Laquelle vibrait le mieux ?\n<- version 1   -> version 2   v pareil   B: revoir"
	case tuneHint:
		return n + " - La version choisie vibrait :\n<- pas assez   A: bien   -> trop   B: retour"
	}
	return "Seance terminee, merci ! (Echap pour quitter)"
}
