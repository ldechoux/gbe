package menusound

import (
	"encoding/binary"
	"math"
	"testing"
)

func samples(k Kind) []int16 {
	b := PCM(k)
	s := make([]int16, len(b)/4)
	for i := range s {
		s[i] = int16(binary.LittleEndian.Uint16(b[4*i:]))
		if r := int16(binary.LittleEndian.Uint16(b[4*i+2:])); r != s[i] {
			panic("the sides differ")
		}
	}
	return s
}

// Every sound is short, soft, and ends in silence.
func TestSounds(t *testing.T) {
	for k := range kinds {
		s := samples(k)
		ms := len(s) * 1000 / Rate
		if ms < 10 || ms > 400 {
			t.Errorf("sound %d lasts %d ms", k, ms)
		}
		peak := 0.0
		for _, v := range s {
			peak = max(peak, math.Abs(float64(v)))
		}
		if db := 20 * math.Log10(peak/32767); db > -8 || db < -30 {
			t.Errorf("sound %d peaks at %.1f dB", k, db)
		}
		if last := s[len(s)-1]; last != 0 {
			t.Errorf("sound %d ends on %d, not silence", k, last)
		}
	}
}

// The tick of Move is an A6: 1760 Hz, counted from its rising edges.
func TestPitch(t *testing.T) {
	s := samples(Move)
	var rises []int
	for i := 1; i < len(s); i++ {
		if s[i-1] < 0 && s[i] >= 0 {
			rises = append(rises, i)
		}
	}
	periods := float64(rises[len(rises)-1]-rises[0]) / float64(len(rises)-1)
	if f := Rate / periods; math.Abs(f-1760) > 30 {
		t.Errorf("Move at %.0f Hz, want 1760", f)
	}
	if f := frequency("C#6"); math.Abs(f-1108.73) > 0.01 {
		t.Errorf("C#6 = %.2f Hz", f)
	}
}
