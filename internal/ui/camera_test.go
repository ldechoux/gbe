package ui

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/webcam"
)

// writePicture writes a w x h PNG, black at the left of x = split, white
// from it.
func writePicture(t *testing.T, path string, w, h, split int) string {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := range h {
		for x := split; x < w; x++ {
			img.SetGray(x, y, color.Gray{0xFF})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// The picture is cut to its middle at 8:7, shrunk by averaging, and
// flipped for the mirror.
func TestSensorImage(t *testing.T) {
	// 512x224: the middle 256 columns are kept, black then white from 256.
	img := image.NewGray(image.Rect(0, 0, 512, 224))
	for y := range 224 {
		for x := 256; x < 512; x++ {
			img.SetGray(x, y, color.Gray{0xFF})
		}
		img.SetGray(129, y, color.Gray{0xFF}) // with column 128, sensor pixel 0
	}
	out := sensorImage(img, false)
	for _, c := range []struct {
		x    int
		want byte
	}{{0, 0x7F}, {1, 0}, {63, 0}, {64, 0xFF}, {127, 0xFF}} {
		if got := out[50*gb.CameraWidth+c.x]; got != c.want {
			t.Errorf("pixel %d: %d, want %d", c.x, got, c.want)
		}
	}
	flipped := sensorImage(img, true)
	if flipped[127] != 0x7F || flipped[0] != 0xFF {
		t.Errorf("mirror: %d at the right, %d at the left", flipped[127], flipped[0])
	}
	// A picture smaller than the sensor is stretched.
	small := image.NewGray(image.Rect(0, 0, 16, 14))
	small.SetGray(15, 13, color.Gray{0xFF})
	if out := sensorImage(small, false); out[len(out)-1] != 0xFF || out[0] != 0 {
		t.Error("a small picture is not stretched")
	}
}

// cameraGame is a game running a Game Boy Camera cartridge.
func cameraGame(t *testing.T) *Game {
	t.Helper()
	g := newDropGame(t)
	g.drop(dropped(writeROM(t, filepath.Join(t.TempDir(), "Camera.gb"), "GAMEBOYCAMERA", 0xFC)))
	if !g.hasCamera() {
		t.Fatal("not a camera")
	}
	return g
}

// The sensor sees the picture of the settings, flipped or not, or noise
// without one.
func TestCameraPicture(t *testing.T) {
	g := cameraGame(t)
	var img gb.CameraImage
	if g.cameraPicture(&img) {
		t.Error("a picture without one in the settings")
	}
	g.cfg.CameraImage = writePicture(t, filepath.Join(t.TempDir(), "p.png"), 128, 112, 64)
	if !g.cameraPicture(&img) || img[0] != 0 || img[127] != 0xFF {
		t.Fatalf("picture: %d at the left, %d at the right", img[0], img[127])
	}
	g.cfg.CameraMirror = true
	if !g.cameraPicture(&img) || img[0] != 0xFF {
		t.Error("the mirror does not flip the picture")
	}
	g.cfg.CameraImage = filepath.Join(t.TempDir(), "gone.png")
	if g.cameraPicture(&img) {
		t.Error("a picture from a missing file")
	}
}

// The Camera entry is only there for the camera; its page chooses the
// picture, forgets it and flips it.
func TestCameraPage(t *testing.T) {
	plain := newDropGame(t)
	plain.drop(dropped(writeROM(t, filepath.Join(t.TempDir(), "Zelda.gb"), "ZELDA", 0x03)))
	if slices.Contains(mainEntries(plain), itemCamera) {
		t.Error("a Camera entry for another game")
	}

	g := cameraGame(t)
	picker := &fakePicker{}
	g.picker = picker
	m := &g.menu
	m.show()
	m.cursor = itemControls
	g.actions = menuActions{down: true}
	m.update(g)
	if m.cursor != itemCamera {
		t.Fatalf("Down from Controls: cursor %d, want the camera", m.cursor)
	}
	if v := m.view(g); v.items[v.selected] != "Camera..." {
		t.Errorf("selected %q", v.items[v.selected])
	}
	g.actions = menuActions{ok: true}
	m.update(g)
	if m.page != pageCamera || m.cursor != cameraChoose {
		t.Fatalf("OK on Camera: page %d, cursor %d", m.page, m.cursor)
	}
	forget := slices.Index(cameraEntries(g), cameraForget)
	if v := m.view(g); !v.disabled[forget] || v.footer != "Picture: none, the camera sees noise\nOr drop a picture on the window" {
		t.Errorf("without a picture: forget enabled %v, footer %q", !v.disabled[forget], v.footer)
	}

	// Choose: the dialog opens; the picture chosen is kept.
	m.update(g)
	if len(picker.images) != 1 || !g.picking {
		t.Fatalf("dialogs %q, picking %v", picker.images, g.picking)
	}
	pic := writePicture(t, filepath.Join(t.TempDir(), "me.png"), 64, 56, 32)
	g.picks <- folderPick{dir: pic}
	g.pollFolderPick()
	if g.cfg.CameraImage != pic || g.toast != "The camera sees me.png" {
		t.Fatalf("picture %q, toast %q", g.cfg.CameraImage, g.toast)
	}
	if v := m.view(g); v.disabled[forget] || v.footer != "Picture: me.png\nOr drop a picture on the window" {
		t.Errorf("with a picture: forget disabled %v, footer %q", v.disabled[forget], v.footer)
	}

	// Mirror: OK and Right switch it.
	m.cursor = cameraMirror
	m.update(g) // OK
	if !g.cfg.CameraMirror {
		t.Error("OK on Mirror did not switch it")
	}
	g.actions = menuActions{right: true}
	m.update(g)
	if g.cfg.CameraMirror {
		t.Error("Right on Mirror did not switch it")
	}

	// Forget, then back to the main page on the Camera entry.
	m.cursor = cameraForget
	g.actions = menuActions{ok: true}
	m.update(g)
	if g.cfg.CameraImage != "" || m.cursor != cameraChoose {
		t.Errorf("after Forget: picture %q, cursor %d", g.cfg.CameraImage, m.cursor)
	}
	g.actions = menuActions{back: true}
	m.update(g)
	if m.page != pageMain || m.cursor != itemCamera {
		t.Errorf("Back: page %d, cursor %d", m.page, m.cursor)
	}
	cfg, err := LoadConfig(g.cfgPath)
	if err != nil || cfg.CameraImage != "" || cfg.CameraMirror {
		t.Errorf("saved %+v, %v", cfg, err)
	}
}

// A picture dropped during a camera game is what the camera sees; during
// another game, it is not a game.
func TestDropCameraPicture(t *testing.T) {
	g := cameraGame(t)
	dir := t.TempDir()
	pic := writePicture(t, filepath.Join(dir, "cat.png"), 32, 28, 16)
	g.drop(dropped(pic))
	if g.cfg.CameraImage != pic || g.menu.open {
		t.Errorf("dropped picture: %q, menu %v", g.cfg.CameraImage, g.menu.open)
	}
	bad := filepath.Join(dir, "bad.jpg")
	if err := os.WriteFile(bad, []byte("not a jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	g.drop(dropped(bad))
	if g.cfg.CameraImage != pic || g.toast != "Unreadable picture: bad.jpg" {
		t.Errorf("unreadable picture: %q, toast %q", g.cfg.CameraImage, g.toast)
	}

	plain := newDropGame(t)
	plain.drop(dropped(writeROM(t, filepath.Join(dir, "Zelda.gb"), "ZELDA", 0x03)))
	plain.drop(dropped(pic))
	if plain.cfg.CameraImage != "" || plain.toast == "" {
		t.Errorf("picture dropped on another game: %q, toast %q", plain.cfg.CameraImage, plain.toast)
	}
}

// fakeWebcams are cameras that give a fixed picture: black at the left,
// white at the right.
type fakeWebcams struct {
	list    []webcam.Device
	err     error // of open
	opens   []string
	streams []*fakeStream
}

type fakeStream struct{ closed bool }

func (f *fakeWebcams) devices() ([]webcam.Device, error) { return f.list, nil }

func (f *fakeWebcams) open(id string) (webcamStream, error) {
	f.opens = append(f.opens, id)
	if f.err != nil {
		return nil, f.err
	}
	s := &fakeStream{}
	f.streams = append(f.streams, s)
	return s, nil
}

func (s *fakeStream) Frame(dst **image.Gray) bool {
	img := image.NewGray(image.Rect(0, 0, 64, 56))
	for y := range 56 {
		for x := 32; x < 64; x++ {
			img.SetGray(x, y, color.Gray{0xFF})
		}
	}
	*dst = img
	return true
}

func (s *fakeStream) Close() { s.closed = true }

// syncUntil calls syncWebcam until the webcam opening is over.
func syncUntil(t *testing.T, g *Game) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		g.syncWebcam()
		if !g.camera.opening {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the webcam does not open")
		}
		time.Sleep(time.Millisecond)
	}
}

// experimentalWebcam sets whether the webcam is experimental for the test,
// whatever the system it runs on.
func experimentalWebcam(t *testing.T, on bool) {
	old := webcamExperimental
	webcamExperimental = on
	t.Cleanup(func() { webcamExperimental = old })
}

var twoWebcams = []webcam.Device{{ID: "obs", Name: "OBSBOT Meet 2 StreamCamera"}, {ID: "lg", Name: "LG UltraFine"}}

// With webcams, the Camera page chooses between the picture and a webcam,
// and which one.
func TestWebcamPage(t *testing.T) {
	experimentalWebcam(t, false)
	g := cameraGame(t)
	g.webcams = &fakeWebcams{list: twoWebcams}
	m := &g.menu
	m.show()
	m.showCamera(g)
	if m.cursor != cameraSource || len(cameraEntries(g)) != cameraItems {
		t.Fatalf("cursor %d, entries %v", m.cursor, cameraEntries(g))
	}
	v := m.view(g)
	if v.items[cameraSource] != "Source    < picture >" || !v.disabled[cameraDevice] {
		t.Errorf("picture: %q, webcam entry enabled %v", v.items[cameraSource], !v.disabled[cameraDevice])
	}
	g.actions = menuActions{down: true}
	m.update(g)
	if m.cursor != cameraChoose {
		t.Errorf("Down skipped to %d, want Choose", m.cursor)
	}

	m.cursor = cameraSource
	g.actions = menuActions{right: true}
	m.update(g)
	if g.cfg.CameraSource != cameraWebcam {
		t.Fatal("Right on Source did not choose the webcam")
	}
	m.cursor = cameraDevice
	var names []string
	for range 3 {
		names = append(names, m.view(g).items[cameraDevice])
		m.update(g)
	}
	want := []string{"Webcam    < default >", "Webcam    < OBSBOT Meet 2 Stre >", "Webcam    < LG UltraFine >"}
	if !slices.Equal(names, want) || g.cfg.CameraDevice != "" {
		t.Errorf("webcams %q, then %q", names, g.cfg.CameraDevice)
	}

	// The footer follows the webcam.
	g.cfg.CameraDevice = "obs"
	g.syncWebcam()
	if f := m.view(g).footer; f != "Webcam: opening...\nOr drop a picture on the window" {
		t.Errorf("opening: %q", f)
	}
	syncUntil(t, g)
	if f := m.view(g).footer; f != "Webcam: on\nOr drop a picture on the window" {
		t.Errorf("open: %q", f)
	}

	// On Linux and Windows, it says the webcam is experimental there.
	webcamExperimental = true
	if f := m.view(g).footer; f != "Webcam: on\nWebcam still experimental on this system\nOr drop a picture on the window" {
		t.Errorf("experimental: %q", f)
	}
	g.cfg.CameraSource = cameraImage
	if f := m.view(g).footer; f != "Picture: none, the camera sees noise\nOr drop a picture on the window" {
		t.Errorf("experimental, the picture: %q", f)
	}
}

// The webcam is open while a camera game runs on it: the sensor sees its
// pictures. It closes when the settings or the game change.
func TestWebcamStream(t *testing.T) {
	g := cameraGame(t)
	cams := &fakeWebcams{list: twoWebcams}
	g.webcams = cams
	g.syncWebcam()
	if len(cams.opens) != 0 {
		t.Fatal("the webcam opened for the picture")
	}
	g.cfg.CameraSource, g.cfg.CameraMirror = cameraWebcam, true
	syncUntil(t, g)
	var img gb.CameraImage
	if len(cams.opens) != 1 || cams.opens[0] != "" || !g.cameraPicture(&img) || img[0] != 0xFF || img[127] != 0 {
		t.Fatalf("opens %q: picture %d at the left, %d at the right (mirrored)", cams.opens, img[0], img[127])
	}

	g.cfg.CameraDevice = "lg"
	syncUntil(t, g)
	if !cams.streams[0].closed || len(cams.opens) != 2 || cams.opens[1] != "lg" || g.camera.stream != cams.streams[1] {
		t.Fatalf("another webcam: opens %q, first closed %v", cams.opens, cams.streams[0].closed)
	}

	// A picture dropped goes back to the picture; the webcam closes.
	g.drop(dropped(writePicture(t, filepath.Join(t.TempDir(), "p.png"), 16, 14, 8)))
	g.syncWebcam()
	if g.cfg.CameraSource != cameraImage || !cams.streams[1].closed || g.camera.stream != nil {
		t.Fatalf("after a dropped picture: source %q, closed %v", g.cfg.CameraSource, cams.streams[1].closed)
	}

	// Another game closes it too.
	g.cfg.CameraSource = cameraWebcam
	syncUntil(t, g)
	g.drop(dropped(writeROM(t, filepath.Join(t.TempDir(), "Zelda.gb"), "ZELDA", 0x03)))
	g.menu.activate(g) // launch it
	g.syncWebcam()
	if g.hasCamera() || !cams.streams[2].closed {
		t.Errorf("another game: camera %v, webcam closed %v", g.hasCamera(), cams.streams[2].closed)
	}
}

// A webcam that does not open says why, once, and is tried again only
// when the settings change.
func TestWebcamDenied(t *testing.T) {
	experimentalWebcam(t, false)
	g := cameraGame(t)
	cams := &fakeWebcams{list: twoWebcams, err: webcam.ErrDenied}
	g.webcams = cams
	g.cfg.CameraSource = cameraWebcam
	syncUntil(t, g)
	if g.toast != "Camera access denied (system settings)" {
		t.Errorf("toast %q", g.toast)
	}
	for range 5 {
		g.syncWebcam()
	}
	if len(cams.opens) != 1 || g.camera.opening {
		t.Errorf("tried %d times", len(cams.opens))
	}
	var img gb.CameraImage
	if g.cameraPicture(&img) {
		t.Error("a picture without the webcam")
	}
	if f := cameraFooter(g); f != "Webcam: unavailable\nOr drop a picture on the window" {
		t.Errorf("footer %q", f)
	}

	cams.err = webcam.ErrNoDevice
	g.cfg.CameraDevice = "lg"
	syncUntil(t, g)
	if len(cams.opens) != 2 || g.toast != "No webcam found" {
		t.Errorf("another webcam: tried %d times, toast %q", len(cams.opens), g.toast)
	}
	cams.err = errors.New("busy")
	g.cfg.CameraSource = cameraImage
	g.syncWebcam()
	g.cfg.CameraSource = cameraWebcam
	syncUntil(t, g)
	if len(cams.opens) != 3 || g.toast != "The webcam cannot be opened" {
		t.Errorf("chosen again: tried %d times, toast %q", len(cams.opens), g.toast)
	}
}
