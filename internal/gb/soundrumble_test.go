package gb

import (
	"math"
	"testing"
)

// The sounds of shocks shake, the others do not; the louder and lower, the
// harder.
func TestSoundRumble(t *testing.T) {
	for _, c := range []struct {
		name      string
		nr51      byte
		noise     byte // NR42, NR43: channel 4 triggered when NR42 is not 0
		noiseFreq byte
		sweep     byte // NR10: channel 1 triggered at full volume when not 0
		want      float64
	}{
		{"silence", 0xFF, 0, 0, 0, 0},
		{"explosion", 0xFF, 0xF0, 0x71, 0, 1},
		{"explosion on one side", 0x0F, 0xB0, 0x71, 0, 0.3074},
		{"quiet explosion", 0xFF, 0x70, 0x71, 0, 0},
		{"explosion not sent out", 0x00, 0xF0, 0x71, 0, 0},
		{"hi-hat", 0xFF, 0xF0, 0x00, 0, 0},
		{"fast sweep", 0xFF, 0, 0, 0x17, 0.5},
		{"slow sweep", 0xFF, 0, 0, 0x71, 0},
		{"no sweep", 0xFF, 0, 0, 0x08, 0},
		{"both", 0x0F, 0xB0, 0x71, 0x17, 0.8074},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := newTestGB(t)
			a := g.APU
			a.write(0xFF24, 0x77)
			a.write(0xFF25, c.nr51)
			if c.noise != 0 {
				a.write(0xFF21, c.noise)
				a.write(0xFF22, c.noiseFreq)
				a.write(0xFF23, 0x80)
			}
			if c.sweep != 0 {
				triggerCh1(a, c.sweep, 0x100)
			}
			if got := g.SoundRumble(); math.Abs(got-c.want) > 1e-3 {
				t.Errorf("rumble %.4f, want %.4f", got, c.want)
			}
		})
	}

	// A silent channel or a powered-off APU does not shake.
	g := newTestGB(t)
	a := g.APU
	a.write(0xFF21, 0xF0)
	a.write(0xFF22, 0x71)
	a.write(0xFF23, 0x80)
	a.ch[3].enabled = false
	if got := g.SoundRumble(); got != 0 {
		t.Errorf("silent channel 4: rumble %v", got)
	}
	a.ch[3].enabled = true
	a.write(0xFF26, 0)
	if got := g.SoundRumble(); got != 0 {
		t.Errorf("APU off: rumble %v", got)
	}
}
