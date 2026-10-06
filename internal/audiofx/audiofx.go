// Package audiofx shapes the sound of the console for the listener, after
// the emulation: how the stereo reaches the ears, and the tone. It works on
// the interleaved stereo int16 samples of the APU, in place, and does not
// change the emulation itself.
package audiofx

import (
	"math"
	"strings"
)

// The stereo modes, in the order of the menu.
const (
	// Stereo leaves the sides as the console mixes them.
	Stereo = "stereo"
	// Headphones lets each ear hear a little of the other side, softened
	// and late, as it does from loudspeakers in a room: a channel the game
	// plays on one side only is less tiring in headphones (crossfeed, after
	// Bauer).
	Headphones = "headphones"
	// Mono plays the sum of both sides on each, as the speaker of the
	// console does.
	Mono = "mono"
)

// The tone filters, in the order of the menu.
const (
	Off = "off"
	// Soft and Warm round off the treble of the square waves, a little and
	// more.
	Soft = "soft"
	Warm = "warm"
	// Speaker sounds like the small speaker of the console: in mono, with
	// neither bass nor treble.
	Speaker = "speaker"
)

// StereoModes and Filters list the IDs of the stereo modes and the filters.
var (
	StereoModes = []string{Stereo, Headphones, Mono}
	Filters     = []string{Off, Soft, Warm, Speaker}
)

// ID returns the ID of ids that s names, ignoring case.
func ID(ids []string, s string) (string, bool) {
	for _, id := range ids {
		if strings.EqualFold(id, s) {
			return id, true
		}
	}
	return "", false
}

// IsMono reports whether the sound comes out in mono with these settings:
// the speaker filter is mono whatever the stereo mode.
func IsMono(stereo, filter string) bool { return stereo == Mono || filter == Speaker }

// Chain applies a stereo mode then a tone filter to the samples.
type Chain struct {
	rate           float64
	stereo, filter string
	cross          crossfeed
	tone           []biquad // run on both sides, in order
	state          [][2]biquadState
}

// New returns a chain for samples at rate Hz, with the given stereo mode and
// filter (IDs from StereoModes and Filters; unknown ones do nothing).
func New(rate float64, stereo, filter string) *Chain {
	c := &Chain{rate: rate}
	c.Set(stereo, filter)
	return c
}

// Set changes the stereo mode and the filter. A filter that changes starts
// from silence.
func (c *Chain) Set(stereo, filter string) {
	if stereo != c.stereo {
		c.stereo = stereo
		c.cross = newCrossfeed(c.rate, 700, 4.5)
	}
	if filter == c.filter {
		return
	}
	c.filter, c.tone = filter, nil
	switch filter {
	case Soft:
		c.tone = []biquad{lowPass(c.rate, 9000)}
	case Warm:
		c.tone = []biquad{lowPass(c.rate, 4500)}
	case Speaker:
		c.tone = []biquad{highPass(c.rate, 350), lowPass(c.rate, 5000)}
	}
	c.state = make([][2]biquadState, len(c.tone))
}

// Active reports whether the chain changes the samples at all.
func (c *Chain) Active() bool {
	return c.stereo == Headphones || IsMono(c.stereo, c.filter) || c.tone != nil
}

// Reset forgets the samples already processed, e.g. when another game
// starts.
func (c *Chain) Reset() {
	c.cross.reset()
	clear(c.state)
}

// Process changes the interleaved stereo samples s in place.
func (c *Chain) Process(s []int16) {
	if !c.Active() {
		return
	}
	mono := IsMono(c.stereo, c.filter)
	for i := 0; i+1 < len(s); i += 2 {
		l, r := float64(s[i]), float64(s[i+1])
		switch {
		case mono:
			l = (l + r) / 2
			r = l
		case c.stereo == Headphones:
			l, r = c.cross.process(l, r)
		}
		for j, f := range c.tone {
			l = f.process(&c.state[j][0], l)
			r = f.process(&c.state[j][1], r)
		}
		s[i], s[i+1] = clamp(l), clamp(r)
	}
}

func clamp(v float64) int16 {
	v = math.Round(v)
	if v > math.MaxInt16 {
		return math.MaxInt16
	}
	if v < math.MinInt16 {
		return math.MinInt16
	}
	return int16(v)
}

// crossfeed sends each side to the other ear through a low-pass filter,
// which also delays it a little, as the head does with loudspeakers, and
// takes the same away from the side itself (after Bauer, as bs2b does).
// Each side gets feed times the difference of the low-passed sides: a sound
// in the middle has none, so it keeps its level at every frequency, and a
// sound on one side reaches the other ear in the bass only.
type crossfeed struct {
	a, feed float64    // the low-pass, and how much of it goes across
	lo      [2]float64 // the low-passed sides
}

// newCrossfeed low-passes at cut Hz. In the bass, a sound on one side
// reaches the other ear level dB lower.
func newCrossfeed(rate, cut, level float64) crossfeed {
	g := math.Pow(10, -level/20) // the other ear against the side, in the bass
	return crossfeed{a: 1 - math.Exp(-2*math.Pi*cut/rate), feed: g / (1 + g)}
}

func (c *crossfeed) reset() { c.lo = [2]float64{} }

func (c *crossfeed) process(l, r float64) (float64, float64) {
	c.lo[0] += c.a * (l - c.lo[0])
	c.lo[1] += c.a * (r - c.lo[1])
	d := c.feed * (c.lo[1] - c.lo[0])
	return l + d, r - d
}

// biquad is a second-order filter (Robert Bristow-Johnson's audio EQ
// cookbook), normalized so that a0 is 1.
type biquad struct{ b0, b1, b2, a1, a2 float64 }

type biquadState struct{ x1, x2, y1, y2 float64 }

func (f biquad) process(s *biquadState, x float64) float64 {
	y := f.b0*x + f.b1*s.x1 + f.b2*s.x2 - f.a1*s.y1 - f.a2*s.y2
	s.x2, s.x1, s.y2, s.y1 = s.x1, x, s.y1, y
	return y
}

// lowPass and highPass are Butterworth filters (Q = 1/√2) with a cut at
// freq Hz.
func lowPass(rate, freq float64) biquad {
	w := 2 * math.Pi * freq / rate
	alpha := math.Sin(w) / math.Sqrt2
	c := math.Cos(w)
	a0 := 1 + alpha
	return biquad{(1 - c) / 2 / a0, (1 - c) / a0, (1 - c) / 2 / a0, -2 * c / a0, (1 - alpha) / a0}
}

func highPass(rate, freq float64) biquad {
	w := 2 * math.Pi * freq / rate
	alpha := math.Sin(w) / math.Sqrt2
	c := math.Cos(w)
	a0 := 1 + alpha
	return biquad{(1 + c) / 2 / a0, -(1 + c) / a0, (1 + c) / 2 / a0, -2 * c / a0, (1 - alpha) / a0}
}
