package ui

// The Game Boy Camera: what its sensor sees. It is a webcam, or a picture
// chosen on the Camera page of the menu or dropped on the window; without
// either, the sensor sees noise, as a real one with its lens covered.

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif" // the formats of a picture for the camera
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/webp"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/webcam"
)

// What the camera sees (Config.CameraSource).
const (
	cameraImage  = "" // the picture, the default
	cameraWebcam = "webcam"
)

// cameraExts are the extensions of the pictures the camera can see.
var cameraExts = []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}

// isCameraImage reports whether name is a picture for the camera, by its
// extension.
func isCameraImage(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range cameraExts {
		if ext == e {
			return true
		}
	}
	return false
}

// camera holds what the sensor sees, made from the settings.
type camera struct {
	picture *gb.CameraImage // nil: noise
	path    string          // the file picture was made from
	mirror  bool            // whether it was flipped

	// The webcam: open while a camera game runs on it. It opens on a
	// goroutine, which may wait for the player to allow it.
	devices   []webcam.Device // as listed when the Camera page opened
	supported bool            // whether the system has a backend
	stream    webcamStream
	streamID  string // the device of stream, or of the one opening
	opening   bool
	opened    chan webcamOpened
	failed    *string     // the device that failed, not tried again until the settings change
	frame     *image.Gray // the latest picture of the webcam
}

// webcams opens the cameras of the system: the webcam package in the game,
// a fake in the tests; nil without cameras.
type webcams interface {
	devices() ([]webcam.Device, error)
	open(id string) (webcamStream, error)
}

// webcamStream is an open webcam (webcam.Stream).
type webcamStream interface {
	Frame(dst **image.Gray) bool
	Close()
}

type webcamOpened struct {
	id     string
	stream webcamStream
	err    error
}

type systemWebcams struct{}

func (systemWebcams) devices() ([]webcam.Device, error) { return webcam.Devices() }

func (systemWebcams) open(id string) (webcamStream, error) {
	s, err := webcam.Open(id)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// listWebcams reads the cameras of the system again, for the Camera page.
func (g *Game) listWebcams() {
	c := &g.camera
	c.devices, c.supported = nil, false
	if g.webcams == nil {
		return
	}
	devices, err := g.webcams.devices()
	if errors.Is(err, webcam.ErrUnsupported) {
		return
	}
	if err != nil {
		log.Printf("camera: %v", err)
	}
	c.devices, c.supported = devices, true
}

// syncWebcam opens the webcam of the settings while a camera game runs on
// it, and closes it otherwise. It is called every tick.
func (g *Game) syncWebcam() {
	c := &g.camera
	want := g.webcams != nil && g.hasCamera() && g.cfg.CameraSource == cameraWebcam
	id := g.cfg.CameraDevice
	select {
	case r := <-c.opened:
		c.opening = false
		switch {
		case r.err != nil:
			log.Printf("camera: %v", r.err)
			c.failed = &r.id
			if want && r.id == id {
				g.notifyLong(webcamError(g, r.err))
			}
		case want && r.id == id:
			c.stream = r.stream
		default: // no longer wanted
			r.stream.Close()
		}
	default:
	}
	if c.stream != nil && (!want || c.streamID != id) {
		c.closeWebcam()
	}
	if !want {
		c.failed = nil // tried again when chosen again
		return
	}
	if c.stream != nil || c.opening || c.failed != nil && *c.failed == id {
		return
	}
	if c.opened == nil {
		c.opened = make(chan webcamOpened, 1)
	}
	c.opening, c.streamID = true, id
	go func(w webcams, done chan<- webcamOpened) {
		s, err := w.open(id)
		done <- webcamOpened{id, s, err}
	}(g.webcams, c.opened)
}

// closeWebcam closes the webcam; its light goes off.
func (c *camera) closeWebcam() {
	if c.stream != nil {
		c.stream.Close()
		c.stream = nil
	}
}

// webcamError explains why the webcam did not open.
func webcamError(g *Game, err error) string {
	l := g.tr()
	switch {
	case errors.Is(err, webcam.ErrDenied):
		return l.T("camera.denied")
	case errors.Is(err, webcam.ErrNoDevice):
		return l.T("camera.no_webcam")
	}
	return l.T("camera.webcam_failed")
}

// hasCamera reports whether the running game is a Game Boy Camera.
func (g *Game) hasCamera() bool { return g.gb != nil && g.gb.Cart.IsCamera() }

// attachCamera makes the sensor of the running game, if a camera, see the
// picture of the settings.
func (g *Game) attachCamera() {
	if g.hasCamera() {
		g.gb.Cart.SetCameraSource(g.cameraPicture)
	}
}

// cameraPicture is the gb.CameraSource of the game: the latest picture of
// the webcam, or the picture of the settings, read again when they change.
func (g *Game) cameraPicture(img *gb.CameraImage) bool {
	c := &g.camera
	if g.cfg.CameraSource == cameraWebcam {
		if c.stream == nil || !c.stream.Frame(&c.frame) {
			return false
		}
		*img = *sensorImage(c.frame, g.cfg.CameraMirror)
		return true
	}
	if c.path != g.cfg.CameraImage || c.mirror != g.cfg.CameraMirror {
		c.path, c.mirror, c.picture = g.cfg.CameraImage, g.cfg.CameraMirror, nil
		if c.path != "" {
			src, err := readImage(c.path)
			if err != nil {
				log.Printf("camera: %v", err)
			} else {
				c.picture = sensorImage(src, c.mirror)
			}
		}
	}
	if c.picture == nil {
		return false
	}
	*img = *c.picture
	return true
}

// setCameraImage makes the picture at path the one the camera sees ("" for
// none), once it is known to be readable.
func (g *Game) setCameraImage(path string) {
	if path != "" {
		if _, err := readImage(path); err != nil {
			log.Printf("camera: %v", err)
			g.notifyLong(g.tr().T("camera.unreadable", filepath.Base(path)))
			return
		}
	}
	g.cfg.CameraImage = path
	if path != "" {
		g.cfg.CameraSource = cameraImage // what the player chose to show
	}
	g.saveConfig()
	if path != "" {
		g.notify(g.tr().T("camera.loaded", filepath.Base(path)))
	}
}

// chooseCameraImage opens the file dialog for the picture of the camera.
func (g *Game) chooseCameraImage() {
	start := ""
	if g.cfg.CameraImage != "" {
		start = filepath.Dir(g.cfg.CameraImage)
	}
	g.chooseFile(start, g.tr().T("camera.dialog_title"), g.setCameraImage)
}

func readImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return img, nil
}

// sensorImage is src as the sensor sees it: its middle, cut to the 8:7 of
// the sensor, shrunk to its 128x112 pixels by averaging, in levels of
// gray; flipped left to right when mirror is set.
func sensorImage(src image.Image, mirror bool) *gb.CameraImage {
	const w, h = gb.CameraWidth, gb.CameraHeight
	b := src.Bounds()
	cw, ch := b.Dx(), b.Dy()
	if cw*h > ch*w {
		cw = ch * w / h
	} else {
		ch = cw * h / w
	}
	x0, y0 := b.Min.X+(b.Dx()-cw)/2, b.Min.Y+(b.Dy()-ch)/2
	var out gb.CameraImage
	if cw == 0 || ch == 0 {
		return &out
	}
	for y := range h {
		sy0, sy1 := y0+y*ch/h, y0+max((y+1)*ch/h, y*ch/h+1)
		for x := range w {
			sx0, sx1 := x0+x*cw/w, x0+max((x+1)*cw/w, x*cw/w+1)
			var sum, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					r, g, b, _ := src.At(sx, sy).RGBA()
					sum += (299*uint64(r) + 587*uint64(g) + 114*uint64(b)) / 1000 // luma
					n++
				}
			}
			dx := x
			if mirror {
				dx = w - 1 - x
			}
			out[y*w+dx] = byte(sum / n >> 8)
		}
	}
	return &out
}
