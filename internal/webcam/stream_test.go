package webcam

import (
	"image"
	"testing"
)

// A picture kept by put comes out of Frame, without the padding of its
// rows; Close runs the closer once.
func TestStream(t *testing.T) {
	var s Stream
	var img *image.Gray
	if s.Frame(&img) || img != nil {
		t.Fatal("a picture before the first")
	}
	// 3x2, 4 bytes per row.
	s.put([]byte{1, 2, 3, 0, 4, 5, 6}, 3, 2, 4)
	if !s.Frame(&img) || img.Rect.Dx() != 3 || img.Rect.Dy() != 2 || string(img.Pix) != "\x01\x02\x03\x04\x05\x06" {
		t.Fatalf("picture %v", img)
	}
	first := img
	s.put([]byte{9, 9, 9, 0, 9, 9, 9}, 3, 2, 4)
	if !s.Frame(&img) || img != first || img.Pix[0] != 9 || s.Frames() != 2 {
		t.Errorf("second picture: same image %v, pixel %d, %d pictures", img == first, img.Pix[0], s.Frames())
	}
	s.put([]byte{7, 7}, 2, 1, 2) // another size
	if !s.Frame(&img) || img.Rect.Dx() != 2 {
		t.Errorf("resized: %v", img.Rect)
	}

	closed := 0
	s.closer = func() { closed++ }
	s.Close()
	s.Close()
	if closed != 1 {
		t.Errorf("closed %d times", closed)
	}
}
