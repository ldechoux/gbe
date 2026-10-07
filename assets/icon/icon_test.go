package icon

import "testing"

func TestWindow(t *testing.T) {
	for i, img := range Window() {
		if n := []int{16, 32, 48}[i]; img.Bounds().Dx() != n || img.Bounds().Dy() != n {
			t.Errorf("icon %d is %v, want %dx%d", i, img.Bounds(), n, n)
		}
	}
}
