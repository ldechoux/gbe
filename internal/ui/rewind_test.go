package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// busyProgram keeps changing BGP, so every frame differs.
var busyProgram = []byte{0x3C, 0xE0, 0x47, 0x18, 0xFB} // INC A; LDH (0x47),A; JR -5

func newPlayingGame(t *testing.T) *Game {
	t.Helper()
	g := withGameBoy(t, newTestGame(t, newFakePads()), busyProgram...)
	g.stream = &audioStream{}
	return g
}

func TestRewinderRing(t *testing.T) {
	g := newPlayingGame(t)
	var r rewinder
	if r.step(g.gb) {
		t.Fatal("step on an empty history must fail")
	}

	var snaps [][]byte
	for i := range rewindEvery * 3 {
		g.gb.RunFrame()
		r.record(g.gb)
		if (i+1)%rewindEvery == 0 {
			snaps = append(snaps, g.gb.Snapshot(nil))
		}
	}
	if r.n != 3 {
		t.Fatalf("%d snapshots after %d frames, want 3", r.n, rewindEvery*3)
	}
	for i := len(snaps) - 1; i >= 0; i-- { // most recent first
		if !r.step(g.gb) {
			t.Fatalf("step %d failed", i)
		}
		if !bytes.Equal(g.gb.Snapshot(nil), snaps[i]) {
			t.Fatalf("step back to snapshot %d restored another state", i)
		}
	}
	if r.step(g.gb) {
		t.Error("step after the oldest snapshot must fail")
	}

	for range rewindEvery * (rewindSlots + 10) {
		g.gb.RunFrame()
		r.record(g.gb)
	}
	if r.n != rewindSlots {
		t.Errorf("%d snapshots, want the ring capped at %d", r.n, rewindSlots)
	}
	r.clear()
	if r.step(g.gb) {
		t.Error("step after clear must fail")
	}
}

func TestAdvanceFastForward(t *testing.T) {
	g := newPlayingGame(t)
	g.cfg.FastForwardSpeed = 4

	g.advance(false, false)
	if g.fps.frames != 1 || g.speedBadge() != "" {
		t.Fatalf("normal tick: %d frames, badge %q", g.fps.frames, g.speedBadge())
	}
	normal := g.stream.buffered()

	g.stream = &audioStream{}
	g.advance(true, false)
	if g.fps.frames != 5 || g.speedBadge() != ">> x4" {
		t.Fatalf("fast tick: %d frames in all, badge %q", g.fps.frames, g.speedBadge())
	}
	// Each tick queues about one frame of audio, whatever the speed.
	if fast := g.stream.buffered(); fast < normal*9/10 || fast > normal*11/10 {
		t.Errorf("fast tick queued %d audio frames, a normal one %d", fast, normal)
	}
}

func TestAdvanceRewind(t *testing.T) {
	g := newPlayingGame(t)
	for range rewindEvery {
		g.advance(false, false)
	}
	want := g.gb.Snapshot(nil)  // the state the rewinder just recorded
	for range rewindEvery - 1 { // not enough for another snapshot
		g.advance(false, false)
	}

	g.advance(true, true) // rewind wins over fast forward
	if !bytes.Equal(g.gb.Snapshot(nil), want) || g.speedBadge() != "<<" {
		t.Fatalf("rewind did not go back to the last snapshot (badge %q)", g.speedBadge())
	}
	queued := g.stream.buffered()
	g.advance(false, true) // odd tick: waits
	g.advance(false, true) // even tick: nothing left, stays put
	if !bytes.Equal(g.gb.Snapshot(nil), want) {
		t.Error("the frame must stay frozen once the history is exhausted")
	}
	if g.stream.buffered() != queued {
		t.Error("rewinding must not queue audio")
	}
	g.advance(false, false)
	if g.speedBadge() != "" || g.rewindTick != 0 {
		t.Errorf("after releasing: badge %q, rewind tick %d", g.speedBadge(), g.rewindTick)
	}
}

func TestFastForwardConfig(t *testing.T) {
	def := DefaultConfig()
	if def.Keys[actionFastForward] != ebiten.KeyTab || def.Keys[actionRewind] != ebiten.KeyBackspace ||
		def.FastForwardSpeed != 4 {
		t.Fatalf("defaults: keys %v speed %d", def.Keys, def.FastForwardSpeed)
	}
	if def.Gamepad[actionFastForward] != std(ebiten.StandardGamepadButtonFrontTopRight) ||
		def.Gamepad[actionRewind] != std(ebiten.StandardGamepadButtonFrontTopLeft) {
		t.Fatalf("gamepad defaults: %v", def.Gamepad)
	}

	// A config written before the actions existed gets their defaults.
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"keys": {"A": "K"}, "fast_forward_speed": 9}`), 0o644)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Keys[actionFastForward] != ebiten.KeyTab || cfg.Gamepad[actionRewind] != def.Gamepad[actionRewind] {
		t.Errorf("old config: keys %v gamepad %v", cfg.Keys, cfg.Gamepad)
	}
	if cfg.FastForwardSpeed != 4 {
		t.Errorf("out of range speed kept: %d", cfg.FastForwardSpeed)
	}

	cfg.FastForwardSpeed = 8
	cfg.Save(path)
	if cfg, _ = LoadConfig(path); cfg.FastForwardSpeed != 8 {
		t.Errorf("saved speed %d, want 8", cfg.FastForwardSpeed)
	}
}

func TestMenuFastForward(t *testing.T) {
	g := newTestGame(t, newFakePads())
	m := &g.menu
	m.show()
	adjust := func(delta int) {
		m.cursor = itemSpeed
		g.actions = menuActions{left: delta < 0, right: delta > 0}
		m.update(g)
	}
	g.cfg.FastForwardSpeed = maxFastForward
	adjust(1)
	if g.cfg.FastForwardSpeed != maxFastForward {
		t.Errorf("speed %d, want clamped to %d", g.cfg.FastForwardSpeed, maxFastForward)
	}
	g.cfg.FastForwardSpeed = minFastForward
	adjust(-1)
	if g.cfg.FastForwardSpeed != minFastForward {
		t.Errorf("speed %d, want clamped to %d", g.cfg.FastForwardSpeed, minFastForward)
	}
	adjust(1)
	if cfg, _ := LoadConfig(g.cfgPath); cfg.FastForwardSpeed != 3 {
		t.Errorf("saved speed %d, want 3", cfg.FastForwardSpeed)
	}
	if got := entryLabel(g, itemSpeed); got != "Fast fwd  < x3 >" {
		t.Errorf("speed entry %q", got)
	}

	// The actions are bound like the buttons, swapping on conflict.
	ff := len(bindingNames()) - 2
	*m = menu{open: true, page: pageControls, cursor: ff, capturing: true}
	m.captureKey(g, Hotkey{Key: ebiten.KeyX}) // A's key
	if g.cfg.Keys[actionFastForward] != ebiten.KeyX || g.cfg.Keys["A"] != ebiten.KeyTab {
		t.Errorf("binding A's key to fast forward: keys %v", g.cfg.Keys)
	}
	if v := m.view(g); v.items[ff] != "Fast fwd X" || v.items[ff+1] != "Rewind   Backspace" {
		t.Errorf("controls lines %q, %q", v.items[ff], v.items[ff+1])
	}
}
