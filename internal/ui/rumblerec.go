package ui

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ldechoux/gbe/internal/gb"
)

// A rumble recording keeps a stretch of play, to tune the rumble detector
// with tools/rumblelab and gbe -rumble-tune (-rumble-record): the save state
// it starts from and the buttons held on each frame. A stretch ends when the game jumps in time
// (rewind, state loaded, reset), another game opens or gbe quits.

// recording is a stretch of play being recorded.
type recording struct {
	title   string // of the game, for the file names
	rom     string // its path
	start   []byte // save state
	frames  int
	cycles  uint64 // of the console after the last frame
	console *gb.GameBoy
	held    [8]int // frame each button was pressed on, -1 if released
	presses []string
}

// rumbleRecorder writes the recordings in dir.
type rumbleRecorder struct {
	dir string
	rec *recording
}

// frame records the frame console just ran (see begin), the buttons held.
func (r *rumbleRecorder) frame(console *gb.GameBoy, down [8]bool) {
	for i, d := range down {
		r.press(i, d)
	}
	r.rec.frames++
	r.rec.cycles = console.Cycles()
}

// begin starts a recording from the state of console before the frame it
// is about to run, unless that frame goes on with the one being recorded.
func (r *rumbleRecorder) begin(console *gb.GameBoy, title, rom string) {
	if r.rec != nil && r.rec.console == console && r.rec.cycles == console.Cycles() {
		return
	}
	r.finish()
	r.rec = &recording{title: title, rom: rom, start: console.SaveState(), console: console, cycles: console.Cycles()}
	for i := range r.rec.held {
		r.rec.held[i] = -1
	}
}

// press follows button i, held or not on the frame being recorded.
func (r *rumbleRecorder) press(i int, down bool) {
	rec := r.rec
	switch {
	case down && rec.held[i] < 0:
		rec.held[i] = rec.frames
	case !down && rec.held[i] >= 0:
		rec.presses = append(rec.presses, fmt.Sprintf("%s:%d-%d", strings.ToLower(gb.Buttons[i].String()), rec.held[i], rec.frames))
		rec.held[i] = -1
	}
}

// finish writes the recording, if any: name.state and name.json.
func (r *rumbleRecorder) finish() {
	rec := r.rec
	r.rec = nil
	if rec == nil || rec.frames < 60 {
		return // under a second: nothing to learn from
	}
	for i := range rec.held {
		r.rec = rec
		r.press(i, false)
		r.rec = nil
	}
	title := strings.Map(func(c rune) rune {
		if c == ' ' || c == '/' || c == '\\' || c == ':' {
			return '_'
		}
		return c
	}, rec.title)
	name := fmt.Sprintf("%s-%s", title, time.Now().Format("20060102-150405"))
	base := filepath.Join(r.dir, name)
	data, err := json.MarshalIndent(map[string]any{
		"rom": rec.rom, "state": name + ".state", "frames": rec.frames,
		"input": strings.Join(rec.presses, ","),
	}, "", "  ")
	if err == nil {
		err = os.MkdirAll(r.dir, 0o755)
	}
	if err == nil {
		err = os.WriteFile(base+".state", rec.start, 0o644)
	}
	if err == nil {
		err = os.WriteFile(base+".json", data, 0o644)
	}
	if err != nil {
		log.Printf("rumble recording: %v", err)
		return
	}
	log.Printf("rumble recording: %s.json, %d frames", base, rec.frames)
}
