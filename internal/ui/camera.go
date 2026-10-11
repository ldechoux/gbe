package ui

// The Game Boy Camera: what its sensor sees. It is a picture chosen on the
// Camera page of the menu, or dropped on the window; without one, the
// sensor sees noise, as a real one with its lens covered.

import (
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

// camera holds the picture the sensor sees, made from the settings.
type camera struct {
	picture *gb.CameraImage // nil: noise
	path    string          // the file picture was made from
	mirror  bool            // whether it was flipped
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

// cameraPicture is the gb.CameraSource of the game: the picture of the
// settings, read again when they change.
func (g *Game) cameraPicture(img *gb.CameraImage) bool {
	c := &g.camera
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
