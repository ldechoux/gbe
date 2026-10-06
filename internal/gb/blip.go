package gb

import "math"

// The sound channels are square waves, wave samples and noise: their level
// jumps at precise T-cycles, so their harmonics go far above what 48 kHz can
// hold. Averaging the level over each output sample filters them poorly, and
// those above half the sample rate fold back into the audible range as
// whistles that do not follow the music (aliasing).
//
// The APU instead adds each jump of a level as a band-limited step, where it
// happens between two output samples (as blip_buf does): the jump is spread
// over a few output samples by a windowed sinc, which keeps the frequencies
// below about 20 kHz and removes those that would fold back. Everything is
// integer arithmetic, so the output is the same on every architecture.

const (
	blipTaps   = 24   // output samples a jump is spread over
	blipPhases = 1024 // positions of a jump between two output samples
	blipShift  = 15   // fixed point of the kernel: each row sums to 1<<blipShift
	blipSize   = 256  // buffer: the samples being made move back to its start at its end

	// blipCutoff is the cutoff of the sinc, relative to the sample rate (0.43
	// of 48 kHz: 20.6 kHz), and blipBeta the shape of its Kaiser window: the
	// response is flat up to 15 kHz (-0.6 dB at 18 kHz), and the
	// frequencies that would fold back below 22 kHz lose at least 80 dB.
	blipCutoff = 0.43
	blipBeta   = 8
)

// blipKernel holds, for each position of a jump between two output samples,
// how much of it each of the next blipTaps output samples gets, as the
// derivative of a band-limited step: the samples are the running sum of what
// the jumps added.
var blipKernel = newBlipKernel()

func newBlipKernel() (k [blipPhases + 1][blipTaps]int32) {
	const half = blipTaps / 2
	i0 := func(x float64) float64 { // modified Bessel function of order 0
		s, t := 1.0, 1.0
		for n := 1; n < 30; n++ {
			q := x / 2 / float64(n)
			t *= q * q
			s += t
		}
		return s
	}
	for p := range k {
		var row [blipTaps]float64
		sum := 0.0
		for i := range row {
			// Time from the jump to output sample i, which is half a kernel
			// late: the samples come out blipTaps/2 samples later.
			t := float64(i-half) + 1 - float64(p)/blipPhases
			x := math.Pi * 2 * blipCutoff * t
			v := 1.0
			if x != 0 {
				v = math.Sin(x) / x
			}
			r := t / half
			if r*r < 1 {
				v *= i0(blipBeta * math.Sqrt(1-r*r))
			} else {
				v = 0
			}
			row[i] = v
			sum += v
		}
		// Each row sums to exactly 1<<blipShift, so that a jump adds its
		// whole height once its samples are out: the error of the rounding
		// goes to the largest tap.
		total, peak := int32(0), 0
		for i, v := range row {
			k[p][i] = int32(math.Round(v / sum * (1 << blipShift)))
			total += k[p][i]
			if k[p][i] > k[p][peak] {
				peak = i
			}
		}
		k[p][peak] += 1<<blipShift - total
	}
	return k
}

// blip turns the jumps of the left and right levels into band-limited
// output samples. Both sides are worked out at once, in the two halves of
// an int64 (the left one in the low 32 bits): (l + r<<32) * v is l*v +
// (r*v)<<32, so a single multiplication serves both. The halves wrap around
// like two int32, which the sums never reach.
type blip struct {
	buf   [blipSize]int64 // what the jumps add to the next output samples
	pos   int             // the output sample being made, in buf; at most blipSize-blipTaps
	sum   int64           // the running sums: the output, << blipShift
	level [2]int          // the levels the jumps went to
}

// pack puts l and r in the two halves of an int64.
func pack(l, r int32) int64 { return int64(l) + int64(r)<<32 }

// unpack takes l and r back from the two halves of an int64.
func unpack(v int64) (l, r int32) {
	l = int32(v)
	return l, int32((v - int64(l)) >> 32)
}

// set makes the levels jump to l and r, at phase (0 to blipPhases) between
// the output sample being made and the next one. A jump is at most
// 2*mixScale, so a whole output sample of them stays far below 1<<31.
func (b *blip) set(l, r, phase int) {
	dl, dr := l-b.level[0], r-b.level[1]
	if dl == 0 && dr == 0 {
		return
	}
	b.level = [2]int{l, r}
	d := pack(int32(dl), int32(dr))
	s := (*[blipTaps]int64)(b.buf[b.pos:])
	for i, v := range &blipKernel[phase] {
		s[i] += d * int64(v)
	}
}

// next returns the output sample being made, << blipShift, and moves to the
// next one.
func (b *blip) next() (l, r int32) {
	b.sum += b.buf[b.pos]
	b.buf[b.pos] = 0
	b.pos++
	if b.pos > blipSize-blipTaps {
		// Keep room for a whole kernel after pos: the samples being made
		// move back to the start, the rest of the buffer is zero.
		n := copy(b.buf[:], b.buf[b.pos:])
		clear(b.buf[n:])
		b.pos = 0
	}
	return unpack(b.sum)
}

// reset makes the output levels right away, without a jump: after a state
// that did not hold the samples being made was loaded.
func (b *blip) reset(l, r int) {
	*b = blip{level: [2]int{l, r}, sum: pack(int32(l)<<blipShift, int32(r)<<blipShift)}
}

// sync saves the samples being made, the only ones not zero, from pos.
func (b *blip) sync(c *codec) {
	if c.loading() {
		*b = blip{}
	}
	c.i64s(b.buf[b.pos : b.pos+blipTaps])
	c.i64(&b.sum)
	c.int(&b.level[0])
	c.int(&b.level[1])
}
