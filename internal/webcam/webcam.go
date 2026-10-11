// Package webcam reads the pictures of a webcam, in levels of gray: what
// the sensor of the Game Boy Camera needs. It is pure Go, as the rest of
// gbe: it calls the video API of the system, AVFoundation on macOS
// (through purego), Video4Linux2 on Linux and Media Foundation on
// Windows.
package webcam

import (
	"errors"
	"image"
	"sync"
)

// The size of picture asked for, when the camera offers a choice: enough
// for the 128x112 of the Game Boy Camera, and light to convert.
const (
	wantWidth  = 640
	wantHeight = 480
)

// Device is a camera of the system.
type Device struct {
	ID   string // stable from one launch to the next
	Name string // as the system shows it
}

var (
	// ErrUnsupported is returned on the systems without a backend yet.
	ErrUnsupported = errors.New("webcam: not supported on this system")
	// ErrDenied is returned when the player, or the system, refuses gbe
	// the camera.
	ErrDenied = errors.New("webcam: access denied")
	// ErrNoDevice is returned without a camera, or when the one asked for
	// is gone.
	ErrNoDevice = errors.New("webcam: no camera")
)

// Stream is an open camera. Its pictures come on a thread of the system;
// Frame gives the latest.
type Stream struct {
	mu     sync.Mutex
	frame  *image.Gray // the latest picture, nil before the first
	seq    uint64      // pictures received
	closer func()
}

// put keeps a picture of w x h levels of gray, stride bytes per row from
// pix, read before put returns.
func (s *Stream) put(pix []byte, w, h, stride int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.frame == nil || s.frame.Rect.Dx() != w || s.frame.Rect.Dy() != h {
		s.frame = image.NewGray(image.Rect(0, 0, w, h))
	}
	for y := range h {
		copy(s.frame.Pix[y*w:(y+1)*w], pix[y*stride:y*stride+w])
	}
	s.seq++
}

// Frame copies the latest picture into dst, resized to it if needed, and
// reports whether there was one.
func (s *Stream) Frame(dst **image.Gray) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.frame == nil {
		return false
	}
	if *dst == nil || (*dst).Rect != s.frame.Rect {
		*dst = image.NewGray(s.frame.Rect)
	}
	copy((*dst).Pix, s.frame.Pix)
	return true
}

// Frames is the number of pictures received since the camera opened.
func (s *Stream) Frames() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seq
}

// Close stops the camera; its light goes off.
func (s *Stream) Close() {
	if s.closer != nil {
		s.closer()
		s.closer = nil
	}
}
