package webcam

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"testing"
	"time"
)

// A real camera, with GBE_WEBCAM=1 (the system may ask for the
// permission): it lists the cameras, opens the default one, and writes
// a picture to $GBE_WEBCAM_OUT when set.
func TestRealCamera(t *testing.T) {
	if os.Getenv("GBE_WEBCAM") == "" {
		t.Skip("set GBE_WEBCAM=1 to use the camera")
	}
	devices, err := Devices()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range devices {
		fmt.Fprintf(os.Stderr, "camera: %s (%s)\n", d.Name, d.ID)
	}
	id := os.Getenv("GBE_WEBCAM_ID")
	s, err := Open(id)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	deadline := time.Now().Add(10 * time.Second)
	for s.Frames() < 30 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	var img *image.Gray
	if !s.Frame(&img) {
		t.Fatal("no picture")
	}
	t.Logf("%d pictures of %v", s.Frames(), img.Rect.Size())
	if out := os.Getenv("GBE_WEBCAM_OUT"); out != "" {
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
}
