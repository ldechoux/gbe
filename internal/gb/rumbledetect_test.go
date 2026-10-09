package gb

import "testing"

// detectorGB is a console looping on itself, with the rumble detector on
// and the sound sent full volume to both sides.
func detectorGB(t *testing.T) *GameBoy {
	t.Helper()
	g := newTestGB(t, 0x18, 0xFE) // JR -2
	g.GuessRumble(true)
	g.TraceRumble(true)
	g.APU.write(0xFF24, 0x77)
	g.APU.write(0xFF25, 0xFF)
	return g
}

// noise triggers channel 4 with NR42 and NR43.
func noise(g *GameBoy, nr42, nr43 byte) {
	g.APU.write(0xFF21, nr42)
	g.APU.write(0xFF22, nr43)
	g.APU.write(0xFF23, 0x80)
}

// sweep triggers channel 1 at full volume, slowly fading, with NR10.
func sweep(g *GameBoy, nr10 byte) {
	g.APU.write(0xFF10, nr10)
	g.APU.write(0xFF12, 0xF3)
	g.APU.write(0xFF13, 0x00)
	g.APU.write(0xFF14, 0x84)
}

// levels runs n frames and returns how hard each shook.
func levels(g *GameBoy, n int) []float64 {
	var l []float64
	for range n {
		g.RunFrame()
		l = append(l, g.GuessedRumble())
	}
	return l
}

func TestRumbleDetectorShocks(t *testing.T) {
	for _, c := range []struct {
		name     string
		play     func(*GameBoy)
		min, max float64 // of the strongest frame
	}{
		{"explosion: loud, low, long noise", func(g *GameBoy) { noise(g, 0xF5, 0x70) }, 0.8, 1},
		{"hi-hat: high noise", func(g *GameBoy) { noise(g, 0xF1, 0x00) }, 0, 0},
		{"quiet noise", func(g *GameBoy) { noise(g, 0x21, 0x70) }, 0, 0},
		{"falling sweep", func(g *GameBoy) { sweep(g, 0x1A) }, 0.5, 1},
		{"plain note", func(g *GameBoy) { sweep(g, 0x00) }, 0, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := detectorGB(t)
			c.play(g)
			peak := 0.0
			for _, v := range levels(g, 10) {
				peak = max(peak, v)
			}
			if peak < c.min || peak > c.max {
				t.Errorf("strongest frame %.2f, want %.2f to %.2f", peak, c.min, c.max)
			}
		})
	}

	// A rising sweep shakes less than the same falling one.
	peak := func(nr10 byte) float64 {
		g := detectorGB(t)
		sweep(g, nr10)
		return levels(g, 1)[0]
	}
	if up, down := peak(0x12), peak(0x1A); up >= down || down == 0 {
		t.Errorf("rising sweep %.2f, falling %.2f", up, down)
	}
}

// A shock is felt at once, for a few frames at least, then fades with its
// sound.
func TestRumbleDetectorPulse(t *testing.T) {
	g := detectorGB(t)
	noise(g, 0xF1, 0x70) // loud and low, but short: 15 steps of 1/64 s
	l := levels(g, 30)
	for f := range pulseMinHold {
		if l[f] < l[0] || l[0] == 0 {
			t.Fatalf("frame %d: %.2f, want the full strength %.2f first", f, l[f], l[0])
		}
	}
	if l[29] != 0 {
		t.Errorf("still shaking after half a second: %v", l)
	}
	for f := pulseMinHold + 1; f < 30; f++ {
		if l[f] > l[f-1] {
			t.Errorf("frame %d stronger than the one before: %v", f, l)
			break
		}
	}
}

// A low drum played on a beat is music: after a few beats, it no longer
// shakes.
func TestRumbleDetectorBeat(t *testing.T) {
	g := detectorGB(t)
	var last []float64
	for beat := range 10 {
		noise(g, 0xA2, 0x70)
		last = levels(g, 13)
		if beat == 0 && last[0] == 0 {
			t.Fatal("the first beat, unheard before, should shake")
		}
	}
	if last[0] != 0 {
		t.Errorf("the tenth beat shakes %.2f", last[0])
	}
	notes := g.DrainRumbleNotes()
	if n := notes[len(notes)-1]; n.Music < 0.5 || n.Channel != 4 {
		t.Errorf("last note %+v, want music", n)
	}
}

// The same note retriggered every frame is a buzz, not a series of shocks.
func TestRumbleDetectorBuzz(t *testing.T) {
	single := detectorGB(t)
	noise(single, 0xF5, 0x70)
	buzz := detectorGB(t)
	var l float64
	for range 20 {
		noise(buzz, 0xF5, 0x70)
		buzz.RunFrame()
		l = buzz.GuessedRumble()
	}
	if s := levels(single, 1)[0]; l >= s/2 {
		t.Errorf("buzz %.2f, single shock %.2f", l, s)
	}
}

// When a routine writes the melody of channel 2 and channel 1, and others
// channel 1 but not channel 2, the first plays music and the others
// effects.
func TestRumbleDetectorWriters(t *testing.T) {
	g := detectorGB(t)
	d := g.guess
	d.writers[0x4000] = &writer{channels: 1<<0 | 1<<1 | 1<<2}
	d.writers[0x6000] = &writer{channels: 1<<0 | 1<<2} // effects on the wave too
	d.writers[0x5000] = &writer{channels: 1 << 0}
	s := &sigStats{last: -1e9}
	if m, e, w := d.music(0x4000, 0, s, 0), d.music(0x5000, 0, s, 0), d.music(0x6000, 0, s, 0); m < 0.5 || e >= 0.5 || w >= 0.5 {
		t.Errorf("music writer %.2f, effect writers %.2f and %.2f", m, e, w)
	}
}

// The detector only watches: the console runs the same with or without
// it, and it starts over after a rewind.
func TestRumbleDetectorWatchOnly(t *testing.T) {
	a, b := detectorGB(t), detectorGB(t)
	b.GuessRumble(false)
	for _, g := range []*GameBoy{a, b} {
		noise(g, 0xF5, 0x70)
		levels(g, 5)
	}
	if string(a.Snapshot(nil)) != string(b.Snapshot(nil)) {
		t.Error("the detector changed the state of the console")
	}
	if b.GuessedRumble() != 0 {
		t.Error("shaking with the detector off")
	}
	snap := a.Snapshot(nil)
	noise(a, 0xF5, 0x70)
	levels(a, 1)
	if err := a.Restore(snap); err != nil || a.GuessedRumble() != 0 {
		t.Errorf("after a rewind: %.2f %v", a.GuessedRumble(), err)
	}
}

// Restored with what it had learned, the detector judges a moment played
// again as the first time.
func TestRumbleMemory(t *testing.T) {
	g := detectorGB(t)
	for range 5 {
		noise(g, 0xA2, 0x70) // a beat, learned as music
		levels(g, 13)
	}
	snap, mem := g.Snapshot(nil), g.RumbleMemory()
	play := func() float64 {
		noise(g, 0xF5, 0x60) // a new noise: a shock
		return levels(g, 1)[0]
	}
	first := play()
	for range 20 {
		play() // heard again and again: familiar
	}
	if err := g.Restore(snap); err != nil {
		t.Fatal(err)
	}
	g.SetRumbleMemory(mem)
	if again := play(); again != first || first == 0 {
		t.Errorf("played again %.2f, first %.2f", again, first)
	}
}
