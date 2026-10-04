package ui

import (
	"encoding/binary"
	"sync"
)

const (
	sampleRate = 48000
	// Buffer levels, in stereo frames.
	targetFill = sampleRate * 50 / 1000
	maxFill    = sampleRate * 150 / 1000
)

// audioStream is a FIFO between the emulator (producer, game loop) and the
// Ebitengine audio player (consumer, its own goroutine). It is read as
// 16-bit little-endian stereo PCM.
type audioStream struct {
	mu   sync.Mutex
	buf  []int16
	last [2]int16
}

func (s *audioStream) push(samples []int16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, samples...)
	if len(s.buf) > maxFill*2 {
		// Way behind (e.g. the window was stalled): drop the oldest audio.
		s.buf = append(s.buf[:0], s.buf[len(s.buf)-targetFill*2:]...)
	}
}

// reset drops the queued audio, and the last frame repeated on underrun:
// they belong to the game left.
func (s *audioStream) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf, s.last = s.buf[:0], [2]int16{}
}

// buffered returns the number of queued stereo frames.
func (s *audioStream) buffered() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.buf) / 2
}

// Read never blocks: on underrun it repeats the last frame, which is silent
// once the emulator's high-pass filter has settled.
func (s *audioStream) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	frames := len(p) / 4
	avail := min(frames, len(s.buf)/2)
	for i := range frames {
		if i < avail {
			s.last[0], s.last[1] = s.buf[i*2], s.buf[i*2+1]
		}
		binary.LittleEndian.PutUint16(p[i*4:], uint16(s.last[0]))
		binary.LittleEndian.PutUint16(p[i*4+2:], uint16(s.last[1]))
	}
	s.buf = append(s.buf[:0], s.buf[avail*2:]...)
	return frames * 4, nil
}

// emulatedRate returns the sample rate the APU should generate at so that
// the buffer stays around targetFill. Ebitengine runs at 60 updates per
// second while the Game Boy runs at ~59.73 Hz, hence the base correction;
// the proportional term (at most ±1%, inaudible) absorbs the remaining drift.
func emulatedRate(fill int) float64 {
	const gbFPS = 4194304.0 / 70224.0
	errRatio := float64(targetFill-fill) / targetFill
	errRatio = max(-1, min(1, errRatio))
	return sampleRate * gbFPS / 60 * (1 + 0.01*errRatio)
}
