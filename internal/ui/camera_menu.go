package ui

// The Camera page of the menu, with a Game Boy Camera only: what its
// sensor sees.

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/menusound"
)

// Camera page entries.
const (
	cameraSource = iota // the picture or a webcam
	cameraDevice        // which webcam
	cameraChoose
	cameraForget
	cameraMirror
	cameraBack
	cameraItems
)

// webcamExperimental is set where the webcam is still experimental: on
// Linux and Windows, where it has not been tried with a real one yet.
var webcamExperimental = runtime.GOOS != "darwin"

// deviceWidth is the longest webcam name the page shows, in characters: a
// longer one scrolls.
const deviceWidth = 18

// cameraEntries lists the entries of the Camera page: those of the webcam
// only where the system has a backend for it.
func cameraEntries(g *Game) []int {
	entries := make([]int, 0, cameraItems)
	for i := range cameraItems {
		if (i == cameraSource || i == cameraDevice) && !g.camera.supported {
			continue
		}
		entries = append(entries, i)
	}
	return entries
}

// cameraDisabled reports whether the Camera page entry i is greyed out:
// the choice of the webcam while the camera sees the picture, or without
// a webcam, and Forget without a picture.
func cameraDisabled(g *Game, i int) bool {
	switch i {
	case cameraDevice:
		return g.cfg.CameraSource != cameraWebcam || len(g.camera.devices) == 0
	case cameraForget:
		return g.cfg.CameraImage == ""
	}
	return false
}

// showCamera opens the Camera page, the cameras of the system listed
// again.
func (m *menu) showCamera(g *Game) {
	g.listWebcams()
	m.page, m.cursor, m.width = pageCamera, cameraEntries(g)[0], 0
}

// moveCamera moves through the entries shown, over those greyed out.
func (m *menu) moveCamera(g *Game, delta int) {
	entries := cameraEntries(g)
	i := max(0, slices.Index(entries, m.cursor))
	for range entries {
		i = (i + len(entries) + delta) % len(entries)
		if !cameraDisabled(g, entries[i]) {
			break
		}
	}
	m.cursor = entries[i]
}

// adjustCamera changes the setting of the selected entry by delta.
func (m *menu) adjustCamera(g *Game, delta int) {
	if cameraDisabled(g, m.cursor) {
		return
	}
	switch m.cursor {
	case cameraSource:
		if g.cfg.CameraSource == cameraWebcam {
			g.cfg.CameraSource = cameraImage
		} else {
			g.cfg.CameraSource = cameraWebcam
		}
	case cameraDevice:
		ids := []string{""} // the default one first
		for _, d := range g.camera.devices {
			ids = append(ids, d.ID)
		}
		g.cfg.CameraDevice = cycle(ids, g.cfg.CameraDevice, delta)
	case cameraMirror:
		g.cfg.CameraMirror = !g.cfg.CameraMirror
	default:
		return
	}
	g.saveConfig()
}

// activateCamera does what OK does on the selected entry.
func (m *menu) activateCamera(g *Game) {
	switch m.cursor {
	case cameraChoose:
		g.menuSound(menusound.Enter)
		g.chooseCameraImage()
	case cameraForget:
		if cameraDisabled(g, cameraForget) {
			g.menuSound(menusound.Refuse)
			return
		}
		g.menuSound(menusound.Change)
		g.setCameraImage("")
		m.cursor = cameraChoose // the entry is greyed out now
	case cameraSource, cameraDevice, cameraMirror:
		m.adjustSounding(g, 1) // OK switches them like Right
	default:
		g.menuSound(menusound.Back)
		m.backToMain()
	}
}

// deviceName is the name of the webcam id, as the page shows it.
func deviceName(g *Game, id string) string {
	for _, d := range g.camera.devices {
		if d.ID == id {
			return marquee(d.Name, deviceWidth, int64(ebiten.Tick()))
		}
	}
	return g.tr().T("camera.device_default")
}

// cameraView is the Camera page: what the sensor sees, and whether it is
// flipped.
func (m *menu) cameraView(g *Game) menuView {
	l := g.tr()
	source := l.T("camera.source_image")
	if g.cfg.CameraSource == cameraWebcam {
		source = l.T("camera.source_webcam")
	}
	labels := []string{
		l.T("camera.source", source),
		l.T("camera.device", deviceName(g, g.cfg.CameraDevice)),
		l.T("camera.choose"),
		l.T("camera.forget"),
		l.T("camera.mirror", onOff(l, g.cfg.CameraMirror)),
		l.T("camera.back"),
	}
	v := menuView{title: l.T("camera.title"), disabled: map[int]bool{}}
	entries := cameraEntries(g)
	for i, e := range entries {
		v.items = append(v.items, labels[e])
		if cameraDisabled(g, e) {
			v.disabled[i] = true
		}
	}
	v.selected = slices.Index(entries, m.cursor)
	v.footer = cameraFooter(g)

	// The panel keeps the size of the longest values.
	help := "\n" + l.T("camera.help")
	v.altItems = []string{l.T("camera.mirror", onOff(l, true)), l.T("camera.mirror", onOff(l, false))}
	v.altFooters = []string{l.T("camera.image_none") + help}
	if g.camera.supported {
		v.altItems = append(v.altItems,
			l.T("camera.source", l.T("camera.source_image")),
			l.T("camera.source", l.T("camera.source_webcam")),
			l.T("camera.device", l.T("camera.device_default")),
			l.T("camera.device", strings.Repeat("W", deviceWidth)))
		if webcamExperimental {
			help = "\n" + l.T("camera.experimental") + help
		}
		v.altFooters = append(v.altFooters,
			l.T("camera.webcam_opening")+help,
			l.T("camera.webcam_on")+help,
			l.T("camera.webcam_off")+help)
	}
	return v
}

// cameraFooter tells what the camera sees: the state of the webcam, or the
// picture.
func cameraFooter(g *Game) string {
	l := g.tr()
	c := &g.camera
	help := "\n" + l.T("camera.help")
	if g.cfg.CameraSource == cameraWebcam {
		if webcamExperimental {
			help = "\n" + l.T("camera.experimental") + help
		}
		switch {
		case c.stream != nil:
			return l.T("camera.webcam_on") + help
		case c.opening:
			return l.T("camera.webcam_opening") + help
		}
		return l.T("camera.webcam_off") + help
	}
	if g.cfg.CameraImage == "" {
		return l.T("camera.image_none") + help
	}
	return l.T("camera.image", marquee(shortDir(filepath.Base(g.cfg.CameraImage)), bootDirWidth, int64(ebiten.Tick()))) + help
}
