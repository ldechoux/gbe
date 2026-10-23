package rendertest

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// recorder is a test that keeps its errors instead of failing; Fatal stops
// it, as testing does.
type recorder struct {
	testing.TB
	errors []string
	fatal  bool
}

func (r *recorder) Helper()      {}
func (r *recorder) Name() string { return "TestRenderScene/dark" }
func (r *recorder) Error(args ...any) {
	r.errors = append(r.errors, fmt.Sprint(args...))
}
func (r *recorder) Fatal(args ...any) {
	r.errors, r.fatal = append(r.errors, fmt.Sprint(args...)), true
	runtime.Goexit()
}
func (r *recorder) Fatalf(format string, args ...any) { r.Fatal(fmt.Sprintf(format, args...)) }

// run checks got against the reference name, as a test would, and says
// what the test reported.
func run(name string, got *image.RGBA) *recorder {
	r := &recorder{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		check(r, name, got)
	}()
	<-done
	return r
}

// scene is a 4x3 picture of a few colors.
func scene() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for y := range 3 {
		for x := range 4 {
			img.SetRGBA(x, y, color.RGBA{uint8(60 * x), uint8(80 * y), 0x40, 0xff})
		}
	}
	return img
}

func TestCheck(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GBE_RENDER_UPDATE", "")
	t.Setenv("GBE_GOLDEN_OUT", "")

	// No reference yet: the test says how to write it.
	if r := run("scene", scene()); !r.fatal || !strings.Contains(r.errors[0], "GBE_RENDER_UPDATE=1") {
		t.Fatalf("without a reference: %q", r.errors)
	}

	// Written, then the same picture passes, and one off by the tolerance.
	t.Setenv("GBE_RENDER_UPDATE", "1")
	if r := run("scene", scene()); len(r.errors) != 0 {
		t.Fatalf("writing the reference: %q", r.errors)
	}
	t.Setenv("GBE_RENDER_UPDATE", "")
	near := scene()
	near.Pix[0] += Tolerance
	for _, img := range []*image.RGBA{scene(), near} {
		if r := run("scene", img); len(r.errors) != 0 {
			t.Errorf("same picture: %q", r.errors)
		}
	}

	// A pixel off: reported, and written with the map of the differences.
	out := t.TempDir()
	t.Setenv("GBE_GOLDEN_OUT", out)
	off := scene()
	off.SetRGBA(2, 1, color.RGBA{0xff, 0xff, 0xff, 0xff})
	r := run("scene", off)
	if len(r.errors) != 1 || !strings.Contains(r.errors[0], "1 pixels differ from the reference, by up to 191") {
		t.Fatalf("a pixel off: %q", r.errors)
	}
	dir := filepath.Join(out, "render", "TestRenderScene_dark")
	if !strings.Contains(r.errors[0], dir) {
		t.Errorf("not said where it is written: %q", r.errors[0])
	}
	if got, err := readPNG(filepath.Join(dir, "scene.png")); err != nil || got.RGBAAt(2, 1) != off.RGBAAt(2, 1) {
		t.Errorf("picture written: %v", err)
	}
	diff, err := readPNG(filepath.Join(dir, "scene-diff.png"))
	if err != nil {
		t.Fatal(err)
	}
	if diff.RGBAAt(2, 1) != (color.RGBA{0xff, 0, 0, 0xff}) || diff.RGBAAt(0, 0) == (color.RGBA{0xff, 0, 0, 0xff}) {
		t.Errorf("diff: %v at the pixel off, %v elsewhere", diff.RGBAAt(2, 1), diff.RGBAAt(0, 0))
	}

	// Another size: reported, without a map.
	t.Setenv("GBE_GOLDEN_OUT", "")
	r = run("scene", image.NewRGBA(image.Rect(0, 0, 2, 2)))
	if len(r.errors) != 1 || !strings.Contains(r.errors[0], "(2,2), want (4,3)") {
		t.Errorf("another size: %q", r.errors)
	}

	// A reference that cannot be written stops the test.
	t.Setenv("GBE_RENDER_UPDATE", "1")
	if err := os.WriteFile("testdata/render/blocked", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if r := run("blocked/scene", scene()); !r.fatal {
		t.Errorf("writing under a file: %q", r.errors)
	}
}

func TestReadPNG(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.png")
	os.WriteFile(bad, []byte("not a png"), 0o644)
	if _, err := readPNG(bad); err == nil || !strings.Contains(err.Error(), bad) {
		t.Errorf("not a PNG: %v", err)
	}

	// A paletted PNG is read as RGBA.
	pal := image.NewPaletted(image.Rect(0, 0, 2, 1), color.Palette{color.Black, color.RGBA{0x10, 0x20, 0x30, 0xff}})
	pal.SetColorIndex(1, 0, 1)
	path := filepath.Join(dir, "pal.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, pal)
	f.Close()
	got, err := readPNG(path)
	if err != nil || got.RGBAAt(1, 0) != (color.RGBA{0x10, 0x20, 0x30, 0xff}) {
		t.Errorf("paletted: %v, %v", got, err)
	}
}

func TestEnabled(t *testing.T) {
	t.Setenv("GBE_RENDER", "")
	if Enabled() {
		t.Error("enabled without GBE_RENDER")
	}
	ran := false
	t.Run("skipped", func(t *testing.T) {
		Skip(t)
		ran = true
	})
	if ran {
		t.Error("not skipped without GBE_RENDER")
	}

	t.Setenv("GBE_RENDER", "1")
	if !Enabled() {
		t.Error("disabled with GBE_RENDER")
	}
	t.Run("run", func(t *testing.T) {
		Skip(t)
		ran = true
	})
	if !ran {
		t.Error("skipped with GBE_RENDER")
	}
}

// The game loop of the tests draws nothing, at the size of the window.
func TestLoop(t *testing.T) {
	var l loop
	l.Draw(nil)
	if w, h := l.Layout(160, 144); w != 160 || h != 144 {
		t.Errorf("layout %dx%d", w, h)
	}
}
