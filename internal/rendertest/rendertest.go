// Package rendertest runs the tests that draw through Ebitengine: the menus,
// the drop screen, the display filters. Drawing needs the game loop of
// Ebitengine, and a display, so these tests only run when GBE_RENDER is set,
// alone (the other tests expect no window):
//
//	GBE_RENDER=1 go test ./internal/ui/...
//
// The CI runs them under a virtual display (xvfb). Their names start with
// TestRender, and they are skipped without GBE_RENDER.
//
// Each test compares what it drew with a reference image, in the
// testdata/render folder of its package. GBE_RENDER_UPDATE=1 writes the
// references instead, to be checked by eye before committing them. When an
// image differs, it is written with a map of the differences to
// $GBE_GOLDEN_OUT/render, which the CI keeps.
package rendertest

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Tolerance is how much a channel of a pixel may differ from the reference,
// from 0 to 255: the GPUs and their drivers (Metal on a Mac, Mesa under
// xvfb) round the shaders a little differently.
const Tolerance = 3

// Enabled reports whether the rendering tests run (GBE_RENDER is set).
func Enabled() bool { return os.Getenv("GBE_RENDER") != "" }

// Main is TestMain for a package with rendering tests: when they are
// enabled, it runs them inside the game loop of Ebitengine, the others
// left out; otherwise it runs the tests as usual.
func Main(m *testing.M) {
	if !Enabled() {
		os.Exit(m.Run())
	}
	flag.Parse()
	if run := flag.Lookup("test.run"); run.Value.String() == "" {
		run.Value.Set("^TestRender")
	}
	l := &loop{m: m}
	ebiten.SetWindowSize(160, 144)
	ebiten.SetWindowTitle("gbe rendering tests")
	ebiten.SetRunnableOnUnfocused(true)
	if err := ebiten.RunGameWithOptions(l, &ebiten.RunGameOptions{InitUnfocused: true}); err != nil {
		fmt.Fprintln(os.Stderr, "rendertest:", err)
		os.Exit(1)
	}
	os.Exit(l.code)
}

// loop runs the tests on its first tick.
type loop struct {
	m    *testing.M
	code int
}

func (l *loop) Update() error {
	l.code = l.m.Run()
	return ebiten.Termination
}

func (l *loop) Draw(*ebiten.Image)         {}
func (l *loop) Layout(w, h int) (int, int) { return w, h }

// Skip skips the test unless the rendering tests run.
func Skip(t testing.TB) {
	t.Helper()
	if !Enabled() {
		t.Skip("rendering test: set GBE_RENDER=1 (see internal/rendertest)")
	}
}

// Check compares img with the reference testdata/render/<name>.png.
func Check(t testing.TB, name string, img *ebiten.Image) {
	t.Helper()
	got := Pixels(img)
	path := filepath.Join("testdata", "render", name+".png")
	if os.Getenv("GBE_RENDER_UPDATE") != "" {
		if err := writePNG(path, got); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := readPNG(path)
	if err != nil {
		t.Fatalf("%v (GBE_RENDER_UPDATE=1 writes the reference)", err)
	}
	diff, n, worst := compare(got, want)
	if n == 0 {
		return
	}
	msg := fmt.Sprintf("%s: %d pixels differ from the reference, by up to %d", name, n, worst)
	if diff == nil {
		msg = fmt.Sprintf("%s: %v, want %v", name, got.Bounds().Size(), want.Bounds().Size())
	}
	if out := os.Getenv("GBE_GOLDEN_OUT"); out != "" {
		dir := filepath.Join(out, "render", strings.ReplaceAll(t.Name(), "/", "_"))
		writePNG(filepath.Join(dir, name+".png"), got)
		if diff != nil {
			writePNG(filepath.Join(dir, name+"-diff.png"), diff)
		}
		msg += " (written to " + dir + ")"
	}
	t.Error(msg)
}

// Pixels reads the pixels of img.
func Pixels(img *ebiten.Image) *image.RGBA {
	out := image.NewRGBA(img.Bounds())
	img.ReadPixels(out.Pix)
	return out
}

// compare counts the pixels of got that differ from want by more than
// Tolerance, and draws them in red over a faded copy of want. diff is nil
// when the sizes differ.
func compare(got, want *image.RGBA) (diff *image.RGBA, n, worst int) {
	if got.Bounds() != want.Bounds() {
		return nil, 1, 255
	}
	diff = image.NewRGBA(want.Bounds())
	for i := 0; i < len(got.Pix); i += 4 {
		d := 0
		for c := range 4 {
			d = max(d, abs(int(got.Pix[i+c])-int(want.Pix[i+c])))
		}
		p := i / 4
		x, y := p%want.Rect.Dx(), p/want.Rect.Dx()
		if d > Tolerance {
			n++
			worst = max(worst, d)
			diff.SetRGBA(x, y, color.RGBA{0xff, 0, 0, 0xff})
		} else {
			fade := func(v uint8) uint8 { return 0xc0 + v/4 }
			diff.SetRGBA(x, y, color.RGBA{fade(want.Pix[i]), fade(want.Pix[i+1]), fade(want.Pix[i+2]), 0xff})
		}
	}
	return diff, n, worst
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func readPNG(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := image.NewRGBA(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	return out, nil
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
