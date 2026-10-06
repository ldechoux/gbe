// Package menusound makes the sounds of the menus, synthesized like the
// Game Boy plays its own: a pulse wave with its duty cycle, a volume
// envelope that steps 64 times a second, and a frequency sweep. They are
// short and soft, and computed once, as 16-bit stereo PCM.
package menusound

import (
	"encoding/binary"
	"math"
)

// Kind is what happened in a menu.
type Kind int

const (
	Move   Kind = iota // the cursor moves to another entry
	Change             // a setting changes
	Enter              // a page opens, or a choice is made
	Back               // back to the previous page, or the menu closes
	Refuse             // nothing to do: a greyed-out entry, a setting at its end
	kinds
)

// note is one note of the pulse channel, or of the wave channel when it has
// a wave.
type note struct {
	name string  // e.g. "C6"
	dur  float64 // seconds
	duty float64 // pulse: part of the period the wave is high: 0.125, 0.25 or 0.5
	vol  int     // 0 to 15
	env  int     // pulse: the volume drops by one every env/64 s (0: it holds)
	// wave holds the 32 4-bit samples the wave channel plays, and fade its
	// volume code at each 1/60 s frame (1: 100%, 2: 50%, 3: 25%, 0: mute;
	// the last one holds): the channel has no envelope, so games change the
	// code as the sound goes on.
	wave *[32]int
	fade []int
}

// pulse is a note of the pulse channel.
func pulse(name string, dur, duty float64, vol, env int) note {
	return note{name: name, dur: dur, duty: duty, vol: vol, env: env}
}

// round is a wave between a triangle and a sine: softer than the pulses.
var round = [32]int{8, 9, 11, 12, 13, 14, 15, 15, 15, 15, 15, 14, 13, 12, 11, 9, 8, 6, 4, 3, 2, 1, 0, 0, 0, 0, 0, 1, 2, 3, 4, 6}

// sounds holds the notes of each kind: bright and short ticks to move and
// change a setting, two soft rising notes of the wave channel to enter, two
// falling ones to go back, and a low tick to refuse.
var sounds = [kinds][]note{
	Move:   {pulse("A6", .018, .125, 6, 0)},
	Change: {pulse("E6", .02, .125, 6, 0)},
	Enter: {
		{name: "C6", dur: .05, vol: 9, wave: &round, fade: []int{2, 2, 2}},
		{name: "G6", dur: .16, vol: 9, wave: &round, fade: []int{2, 2, 2, 3, 3, 3, 3, 3, 0}},
	},
	Back:   {pulse("G6", .03, .25, 7, 0), pulse("C6", .05, .25, 7, 1)},
	Refuse: {pulse("C4", .04, .125, 7, 0)},
}

// Rate is the sample rate of the sounds.
const Rate = 48000

// PCM returns the sound of kind k as 16-bit little-endian stereo samples at
// Rate.
func PCM(k Kind) []byte {
	s := render(sounds[k])
	b := make([]byte, 4*len(s))
	for i, v := range s {
		x := uint16(int16(math.Round(max(-1, min(1, v)) * 32767)))
		binary.LittleEndian.PutUint16(b[4*i:], x)
		binary.LittleEndian.PutUint16(b[4*i+2:], x)
	}
	return b
}

// level is the loudest a note gets, at volume 15: soft beside the games.
const level = 0.25

// oversample is how many times faster than Rate the waves are computed,
// then averaged: the edges of the pulses do not fold back as whistles.
const oversample = 16

func render(notes []note) []float64 {
	var out []float64
	for _, n := range notes {
		freq := frequency(n.name)
		phase := 0.0
		for i := range int(n.dur * Rate) {
			vol := n.vol
			if n.env > 0 {
				vol = max(0, vol-int(float64(i)/Rate*64/float64(n.env)))
			}
			code := 1 // the volume code of the wave channel
			if len(n.fade) > 0 {
				code = n.fade[min(i*60/Rate, len(n.fade)-1)]
			}
			sum := 0.0 // of the channel's output, from 0 to 1
			for range oversample {
				phase += freq / (Rate * oversample)
				phase -= math.Floor(phase)
				switch {
				case n.wave == nil:
					if phase < n.duty {
						sum++
					}
				case code != 0:
					// The code shifts the 4-bit sample right: 100%, 50%, 25%.
					sum += float64(n.wave[int(phase*32)]>>(code-1)) / 15
				}
			}
			// The DAC: the wave between -1 and 1, scaled by the volume.
			out = append(out, (2*sum/oversample-1)*float64(vol)/15*level)
		}
	}
	// The console's output capacitor takes the offset of the pulses away;
	// the last 1 ms fades out, so that the sound does not end on a click.
	hp, prev := 0.0, 0.0
	for i, v := range out {
		hp = 0.995 * (hp + v - prev)
		prev = v
		out[i] = hp
	}
	fade := min(Rate/1000, len(out))
	for i := range fade {
		out[len(out)-1-i] *= float64(i) / float64(fade)
	}
	return out
}

// frequency is the frequency in Hz of a note such as "A4" (440 Hz) or "C#6".
func frequency(name string) float64 {
	semis := map[byte]int{'C': -9, 'D': -7, 'E': -5, 'F': -4, 'G': -2, 'A': 0, 'B': 2}[name[0]]
	i := 1
	if name[1] == '#' {
		semis++
		i++
	}
	octave := int(name[i] - '0')
	return 440 * math.Pow(2, float64(semis)/12+float64(octave-4))
}
