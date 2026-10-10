package gb

import "math"

// Many games show their shocks on the screen too: the screen shakes (a boss
// landing, an earthquake, a ground pound) or flashes (an explosion, a hit).
// The rumble detector watches both, once per frame:
//
//   - Shake: the background scroll on the middle line goes back and forth,
//     a few pixels at a time, changing direction several times within a
//     few frames, without going anywhere. It shakes the gamepad even
//     without a sound.
//   - Flash: the screen gets much lighter or darker from one frame to the
//     next (a quarter or more: the lightning of the Zelda DX intro only
//     takes the mean from 0.24 to 0.34).
//     Menus and scene changes do that too, so a flash never shakes on its
//     own: it only makes the shocks heard about then stronger.

// Screen settings.
const (
	shakeFrames    = 10 // frames the back and forth of a shake is looked for in
	shakeTurns     = 3  // changes of direction within them that make a shake
	shakeMaxStep   = 12 // pixels the scroll moves by in a frame at most while shaking
	shakeFullStep  = 6  // pixels per frame that shake at full strength
	flashJump      = 1.25 // lighter (or darker) by this ratio makes a flash
	flashSteady    = 0.03 // while the frame before moved by less
	flashMin       = 0.05 // and the change is at least
	flashBoostTime = 4 // frames a flash and a shock may be apart
)

// screenWatch is what the detector follows of the screen.
type screenWatch struct {
	scroll   [shakeFrames + 1][2]int // middle line scroll, newest first
	frames   int                     // in scroll so far
	light    [3]float64              // lightness of the last frames, newest first
	flashAt  float64                 // frame of the last flash
	shake    float64                 // of the last frame, 0 to 1
	flashing bool                    // the last frame flashed
}

// ScreenRumble tells what the rumble detector saw on the last frame: how
// hard the screen shook, from 0 to 1, and whether it flashed.
func (g *GameBoy) ScreenRumble() (shake float64, flash bool) {
	if g.guess == nil {
		return 0, false
	}
	return g.guess.screen.shake, g.guess.screen.flashing
}

// watchScreen follows the screen of the frame just run. It reports how hard
// it shakes.
func (d *rumbleDetector) watchScreen(t float64) float64 {
	s := &d.screen
	p := d.g.PPU
	if p.lcdc&0x80 == 0 {
		*s = screenWatch{flashAt: s.flashAt} // the LCD is off: start over
		return 0
	}
	copy(s.scroll[1:], s.scroll[:])
	s.scroll[0] = [2]int{int(p.midSCX), int(p.midSCY)}
	s.frames = min(s.frames+1, len(s.scroll))
	s.shake = 0
	for axis := range 2 {
		s.shake = math.Max(s.shake, s.shaking(axis))
	}

	copy(s.light[1:], s.light[:])
	s.light[0] = d.lightness()
	s.flashing = false
	if s.frames >= len(s.light) && s.flash() {
		s.flashing, s.flashAt = true, t
		// A shock heard just before: stronger.
		for i := range d.pulses {
			if p := &d.pulses[i]; p.active && t-p.start <= flashBoostTime {
				p.strength = math.Min(1, p.strength*(1+d.params().Flash))
			}
		}
	}
	return s.shake * d.params().Shake
}

// shaking tells how hard the scroll on axis (0 for x, 1 for y) shakes, from
// 0 to 1.
func (s *screenWatch) shaking(axis int) float64 {
	turns, sign, net, step := 0, 0, 0, 0
	for k := range s.frames - 1 {
		d := int(int8(s.scroll[k][axis] - s.scroll[k+1][axis])) // wraps around 256
		if d == 0 {
			continue
		}
		if abs(d) > shakeMaxStep {
			return 0 // a jump: a new scene, not a shake
		}
		if sign != 0 && (d > 0) != (sign > 0) {
			turns++
		}
		sign, net, step = d, net+d, max(step, abs(d))
	}
	// Back and forth without going anywhere: not a scroll that follows a
	// player turning around now and then.
	if turns < shakeTurns || abs(net) > step {
		return 0
	}
	return 0.4 + 0.6*math.Min(1, float64(step)/shakeFullStep)
}

// flash tells whether the last frame flashed: much lighter or darker than
// the one before, which had not moved much.
func (s *screenWatch) flash() bool {
	now, before, steady := s.light[0], s.light[1], s.light[2]
	if math.Abs(before-steady) > flashSteady || math.Abs(now-before) < flashMin {
		return false
	}
	lo, hi := math.Min(now, before), math.Max(now, before)
	return hi >= flashJump*math.Max(lo, 0.01)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// lightness tells how light the last frame is, from 0 (black) to 1 (white),
// from one pixel in 16.
func (d *rumbleDetector) lightness() float64 {
	sum, n := 0.0, 0
	if d.g.IsCGB() {
		fb := d.g.ColorFramebuffer()
		for y := 0; y < ScreenHeight; y += 4 {
			for x := 0; x < ScreenWidth; x += 4 {
				c := fb[y*ScreenWidth+x]
				sum += float64(c&31+c>>5&31+c>>10&31) / 93
				n++
			}
		}
	} else {
		fb := d.g.Framebuffer()
		for y := 0; y < ScreenHeight; y += 4 {
			for x := 0; x < ScreenWidth; x += 4 {
				sum += 1 - float64(fb[y*ScreenWidth+x]&3)/3
				n++
			}
		}
	}
	return sum / float64(n)
}
