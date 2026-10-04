package ui

import (
	"encoding/binary"
	"sync"

	"github.com/ldechoux/gbe/internal/gb"
)

const screenPixels = gb.ScreenWidth * gb.ScreenHeight

// frameSource is a frame of the console: its DMG shades or, in Game Boy
// Color mode, its RGB555 colors.
type frameSource struct {
	color  bool
	shades *[screenPixels]byte
	colors *[screenPixels]uint16
}

// consoleFrame is the last complete frame of the console.
func consoleFrame(console *gb.GameBoy) frameSource {
	return frameSource{console.IsCGB(), console.Framebuffer(), console.ColorFramebuffer()}
}

// frameRGBA writes src into dst as RGBA pixels. DMG shades go through pal;
// Game Boy Color frames are RGB555, optionally adjusted to look like the
// original screen (see correctColor).
func frameRGBA(dst []byte, src frameSource, pal *Palette, correct bool) {
	dst = dst[:screenPixels*4]
	if !src.color {
		var shades [4]uint32
		for i, c := range pal.Colors {
			shades[i] = packRGBA(c.R, c.G, c.B)
		}
		// Ranged over as a slice: the array pointer, from a struct, would
		// otherwise be checked for nil at every pixel.
		for i, s := range src.shades[:] {
			binary.LittleEndian.PutUint32(dst[i*4:], shades[s&3])
		}
		return
	}
	lut := colorLUT(correct)[:] // same for the table
	for i, c := range src.colors[:] {
		binary.LittleEndian.PutUint32(dst[i*4:], lut[c&0x7FFF])
	}
}

// packRGBA packs an opaque color as the 4 bytes of an RGBA pixel, read as a
// little-endian uint32.
func packRGBA(r, g, b byte) uint32 {
	return uint32(r) | uint32(g)<<8 | uint32(b)<<16 | 0xFF<<24
}

// colorLUTs holds every RGB555 color converted by rgb555, raw and corrected
// (packed by packRGBA). They are built on first use.
var (
	colorLUTs    [2]*[1 << 15]uint32
	colorLUTOnce [2]sync.Once
)

func colorLUT(correct bool) *[1 << 15]uint32 {
	i := 0
	if correct {
		i = 1
	}
	colorLUTOnce[i].Do(func() {
		lut := new([1 << 15]uint32)
		for c := range lut {
			lut[c] = packRGBA(rgb555(uint16(c), correct))
		}
		colorLUTs[i] = lut
	})
	return colorLUTs[i]
}

// rgb555 converts a CGB color (red in the low bits) to 8-bit components.
func rgb555(c uint16, correct bool) (r, g, b byte) {
	r5, g5, b5 := int(c&0x1F), int(c>>5&0x1F), int(c>>10&0x1F)
	if correct {
		return correctColor(r5, g5, b5)
	}
	scale := func(v int) byte { return byte(v<<3 | v>>2) }
	return scale(r5), scale(g5), scale(b5)
}

// correctColor mimics the CGB LCD, whose colors are paler and bleed into
// each other: games were designed for it and look oversaturated otherwise.
// This is the usual formula from byuu's higan.
func correctColor(r, g, b int) (byte, byte, byte) {
	cr := r*26 + g*4 + b*2
	cg := g*24 + b*8
	cb := r*6 + g*4 + b*22
	return byte(min(960, cr) >> 2), byte(min(960, cg) >> 2), byte(min(960, cb) >> 2)
}

// lcdFrame is the frame shown on screen, converted to RGBA. Draw runs at the
// refresh rate of the display, often more than once per emulated frame, and
// not at all while the menu is open: the frame is only converted again when
// its source changed.
type lcdFrame struct {
	pix []byte

	// What pix was converted from.
	valid   bool
	color   bool
	shades  [screenPixels]byte
	colors  [screenPixels]uint16
	pal     Palette
	correct bool
}

// update converts src into pix, unless it is the frame already there. It
// reports whether pix changed.
func (f *lcdFrame) update(src frameSource, pal *Palette, correct bool) bool {
	same := f.valid && f.color == src.color
	if src.color {
		same = same && f.correct == correct && f.colors == *src.colors
	} else {
		same = same && f.pal == *pal && f.shades == *src.shades
	}
	if same {
		return false
	}
	if f.pix == nil {
		f.pix = make([]byte, screenPixels*4)
	}
	frameRGBA(f.pix, src, pal, correct)
	f.valid, f.color, f.pal, f.correct = true, src.color, *pal, correct
	if src.color {
		f.colors = *src.colors
	} else {
		f.shades = *src.shades
	}
	return true
}
