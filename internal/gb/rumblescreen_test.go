package gb

import "testing"

// screenFrames feeds the detector frames whose middle line is scrolled to
// ys, and returns how hard each shook.
func screenFrames(g *GameBoy, ys ...int) []float64 {
	d := g.guess
	g.PPU.lcdc |= 0x80
	var l []float64
	for _, y := range ys {
		g.PPU.midSCY = byte(y)
		l = append(l, d.watchScreen(d.now()))
	}
	return l
}

func TestScreenShake(t *testing.T) {
	for _, c := range []struct {
		name  string
		ys    []int
		shake bool
	}{
		{"back and forth", []int{0, 2, 0, 2, 0, 2, 0, 2, 0, 2}, true},
		{"around 0, wrapping", []int{0, 254, 0, 254, 0, 254, 0, 254}, true},
		{"steady scroll", []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, false},
		{"turning around once", []int{0, 2, 4, 6, 4, 2, 0, 2, 4, 6}, false},
		{"still", []int{5, 5, 5, 5, 5, 5, 5, 5}, false},
		{"new scenes", []int{0, 100, 0, 100, 0, 100, 0, 100}, false},
	} {
		g := detectorGB(t)
		l := screenFrames(g, c.ys...)
		if got := l[len(l)-1] > 0; got != c.shake {
			t.Errorf("%s: %v", c.name, l)
		}
	}
	// Off, the screen does not shake the gamepad.
	g := detectorGB(t)
	p := testParams
	p.Shake = 0
	g.SetRumbleParams(p)
	if l := screenFrames(g, 0, 2, 0, 2, 0, 2, 0, 2); l[len(l)-1] != 0 {
		t.Errorf("shake off: %v", l)
	}
}

// A flash makes the shocks heard about then stronger, but never shakes on
// its own.
func TestScreenFlash(t *testing.T) {
	light := func(g *GameBoy, shade byte) float64 { // one frame, all pixels of shade
		for i := range g.PPU.front {
			g.PPU.front[i] = shade
		}
		return screenFrames(g, 0)[0]
	}
	shock := func(flash bool) float64 {
		g := detectorGB(t)
		for range 3 {
			light(g, 2)
		}
		if flash {
			light(g, 0) // much lighter
			if _, f := g.ScreenRumble(); !f {
				t.Fatal("no flash seen")
			}
		}
		noise(g, 0xA5, 0x70)
		return g.DrainRumbleNotes()[0].Shock
	}
	if plain, flashed := shock(false), shock(true); flashed <= plain || plain == 0 {
		t.Errorf("shock %.2f, with a flash %.2f", plain, flashed)
	}
	g := detectorGB(t)
	for _, s := range []byte{2, 2, 2, 0, 0, 2} {
		if v := light(g, s); v != 0 {
			t.Errorf("a flash alone shook %.2f", v)
		}
	}
}
