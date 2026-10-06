package audiofx

import (
	"math"
	"slices"
	"testing"
)

const rate = 48000

// gains runs a sine of freq Hz through c, at amplitude l on the left and r
// on the right, and returns the level of each output side in dB relative to
// the amplitude of the louder input side.
func gains(c *Chain, freq, l, r float64) (dbL, dbR float64) {
	const n = rate / 2 // half a second: the filters settle in the first half
	s := make([]int16, 2*n)
	for i := range n {
		v := math.Sin(2 * math.Pi * freq * float64(i) / rate)
		s[2*i], s[2*i+1] = int16(math.Round(l*v)), int16(math.Round(r*v))
	}
	c.Process(s)
	var pl, pr float64
	for i := n / 2; i < n; i++ {
		pl += float64(s[2*i]) * float64(s[2*i])
		pr += float64(s[2*i+1]) * float64(s[2*i+1])
	}
	ref := max(l, r) * max(l, r) / 2 * float64(n-n/2)
	return 10 * math.Log10(pl/ref), 10 * math.Log10(pr/ref)
}

func near(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: %.2f dB, want %.2f ± %.2f", what, got, want, tol)
	}
}

func TestStereoLeavesSamples(t *testing.T) {
	c := New(rate, Stereo, Off)
	s := []int16{1000, -2000, 32767, -32768}
	c.Process(s)
	if s[0] != 1000 || s[1] != -2000 || s[2] != 32767 || s[3] != -32768 || c.Active() {
		t.Errorf("samples changed: %v", s)
	}
}

func TestMono(t *testing.T) {
	c := New(rate, Mono, Off)
	s := []int16{1000, -2000, 300, 301}
	c.Process(s)
	if s[0] != -500 || s[1] != -500 || s[2] != 301 || s[3] != 301 {
		t.Errorf("mono: %v, want both sides at their mean", s)
	}
	if !IsMono(Stereo, Speaker) {
		t.Error("the speaker filter is not mono")
	}
}

// A sound in the middle keeps its level at every frequency; one on a single
// side reaches the other ear only in the bass, about 4.5 dB lower.
func TestHeadphones(t *testing.T) {
	for _, f := range []float64{60, 200, 1000, 5000, 15000} {
		l, r := gains(New(rate, Headphones, Off), f, 10000, 10000)
		near(t, "middle left", l, 0, 0.5)
		near(t, "middle right", r, 0, 0.5)
	}
	l, r := gains(New(rate, Headphones, Off), 60, 10000, 0)
	near(t, "left only, 60 Hz, other side", r-l, -4.5, 0.5)
	l, r = gains(New(rate, Headphones, Off), 8000, 10000, 0)
	if r-l > -20 {
		t.Errorf("left only, 8 kHz: the other side gets %.1f dB, want below -20", r-l)
	}
	near(t, "left only, 8 kHz, own side", l, 0, 2.5)
}

func TestFilters(t *testing.T) {
	for _, c := range []struct {
		filter string
		freq   float64
		want   float64
		tol    float64
	}{
		{Soft, 1000, 0, 0.1}, {Soft, 9000, -3, 0.3}, {Soft, 16000, -16.6, 0.5},
		{Warm, 1000, 0, 0.3}, {Warm, 4500, -3, 0.3}, {Warm, 12000, -20.8, 0.5},
		{Speaker, 100, -21.8, 0.5}, {Speaker, 1500, 0, 1}, {Speaker, 12000, -18.8, 0.5},
	} {
		l, r := gains(New(rate, Stereo, c.filter), c.freq, 10000, 10000)
		near(t, c.filter+" left", l, c.want, c.tol)
		near(t, c.filter+" right", r, c.want, c.tol)
	}
}

// Loud samples are clipped instead of wrapping around, and a chain set back
// to Stereo and Off after other settings leaves the samples alone.
func TestClipAndReset(t *testing.T) {
	if clamp(40000) != 32767 || clamp(-40000) != -32768 || clamp(-0.4) != 0 {
		t.Error("clamp does not clip")
	}
	// A low-pass filter overshoots a full-scale step: the peak is clipped.
	c := New(rate, Stereo, Warm)
	s := make([]int16, 2000)
	for i := 1000; i < len(s); i++ {
		s[i] = 32767
	}
	c.Process(s)
	if peak := slices.Max(s); peak != 32767 || slices.Min(s[1000:]) < 0 {
		t.Errorf("full-scale step: peak %d, minimum %d after it", peak, slices.Min(s[1000:]))
	}
	c.Set(Mono, Speaker)
	c.Reset()
	c.Process(s)
	c.Set(Stereo, Off)
	z := []int16{123, -456}
	c.Process(z)
	if z[0] != 123 || z[1] != -456 {
		t.Errorf("Stereo and Off changed the samples: %v", z)
	}
}

func TestID(t *testing.T) {
	if id, ok := ID(StereoModes, "HeadPhones"); !ok || id != Headphones {
		t.Errorf("ID(HeadPhones) = %q, %v", id, ok)
	}
	if _, ok := ID(Filters, "loud"); ok {
		t.Error("unknown filter accepted")
	}
}
