package gb

import (
	"math"
	"testing"
)

// Each row of the kernel adds a whole jump, whatever its phase.
func TestBlipKernelSums(t *testing.T) {
	for p, row := range blipKernel {
		sum := int32(0)
		for _, v := range row {
			sum += v
		}
		if sum != 1<<blipShift {
			t.Errorf("phase %d: row sums to %d, want %d", p, sum, 1<<blipShift)
		}
	}
}

// A jump reaches its level once its samples are out, and stays there.
func TestBlipStep(t *testing.T) {
	var b blip
	b.set(100, -40, blipPhases/3)
	for i := range 2 * blipTaps {
		l, r := b.next()
		if i >= blipTaps && (l != 100<<blipShift || r != -40<<blipShift) {
			t.Fatalf("sample %d: %d, %d, want %d, %d", i, l, r, 100<<blipShift, -40<<blipShift)
		}
	}
}

// A high note keeps its harmonics, and the ones above 24 kHz do not fold
// back into the audible range: everything heard between the harmonics is
// far below them (it was 26 dB with the average of each sample).
func TestAPUAliasing(t *testing.T) {
	g := newTestGB(t, 0x18, 0xFE) // JR -2: spin forever
	g.APU.SetSampleRate(48000)
	g.Bus.write(0xFF24, 0x77)
	g.Bus.write(0xFF25, 0x22)
	const x = 2011            // 131072 / (2048-x): 3542 Hz
	g.Bus.write(0xFF16, 0x80) // duty 50%
	g.Bus.write(0xFF17, 0xF0)
	g.Bus.write(0xFF18, x&0xFF)
	g.Bus.write(0xFF19, 0x80|x>>8)
	const n = 16384
	var s []float64
	for len(s) < 2*n {
		g.RunFrame()
		d := g.APU.DrainSamples()
		for i := 0; i < len(d); i += 2 {
			s = append(s, float64(d[i]))
		}
	}
	s = s[len(s)-n:] // once the high-pass filter has settled
	for i := range s {
		s[i] *= 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/n) // Hann window
	}

	f0 := 131072.0 / (2048 - x)
	var harmonics, other float64
	for k := 1; float64(k)*48000/n < 20000; k++ {
		var re, im float64
		for i, v := range s {
			a := 2 * math.Pi * float64(k*i%n) / n
			re += v * math.Cos(a)
			im += v * math.Sin(a)
		}
		h := float64(k) * 48000 / n / f0
		if math.Abs(h-math.Round(h))*f0 < 30 {
			harmonics += re*re + im*im
		} else {
			other += re*re + im*im
		}
	}
	if db := 10 * math.Log10(harmonics/other); db < 60 {
		t.Errorf("aliasing %.1f dB below the harmonics, want at least 60", db)
	}
}
