package gb

import (
	"bytes"
	"path/filepath"
	"slices"
	"testing"
)

// stepFrame runs a frame as RunFrame does, but without skipping the quiet
// M-cycles of a halt: one M-cycle per Step.
func stepFrame(g *GameBoy) {
	start := g.Bus.cycles
	g.PPU.frameReady = false
	for !g.PPU.frameReady && g.Bus.cycles-start < CyclesPerFrame {
		g.CPU.Step()
	}
	if g.Cart.rumble {
		g.rumble = g.Cart.motorShare(start, g.Bus.cycles)
	}
}

// Skipping the quiet M-cycles of a halt leaves the machine as running them
// one by one: the whole state, the frame and the sound, after every frame.
func TestHaltSkipSameState(t *testing.T) {
	type run struct {
		name     string
		skip, by func() *GameBoy
		inputs   func(*GameBoy, int)
	}
	var runs []run
	for _, s := range benchScenes {
		for _, m := range benchModels {
			if s.button < 0 {
				continue
			}
			runs = append(runs, run{
				name:   s.name + "/" + m.name,
				skip:   func() *GameBoy { return newBenchROM(t, s, m.model) },
				by:     func() *GameBoy { return newBenchROM(t, s, m.model) },
				inputs: benchInputs,
			})
		}
	}
	// A game that spends most of its time halted, when available.
	sml := filepath.Join(gameDir, "Super_Mario_Land_World_Rev1.gb")
	runs = append(runs, run{
		name:   "sml",
		skip:   func() *GameBoy { return loadROMModel(t, sml, ModelDMG) },
		by:     func() *GameBoy { return loadROMModel(t, sml, ModelDMG) },
		inputs: func(g *GameBoy, f int) { g.SetButton(ButtonStart, f >= 200 && f < 210) },
	})
	for _, r := range runs {
		t.Run(r.name, func(t *testing.T) {
			a, b := r.skip(), r.by()
			var sa, sb []byte
			for f := range 400 {
				r.inputs(a, f)
				r.inputs(b, f)
				a.RunFrame()
				stepFrame(b)
				if !slices.Equal(a.APU.DrainSamples(), b.APU.DrainSamples()) {
					t.Fatalf("frame %d: the sound differs", f)
				}
				sa, sb = a.Snapshot(sa), b.Snapshot(sb)
				if !bytes.Equal(sa, sb) {
					t.Fatalf("frame %d: the state differs", f)
				}
			}
		})
	}
}
