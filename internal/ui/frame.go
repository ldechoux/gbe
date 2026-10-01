package ui

import (
	"github.com/ldechoux/gbe/internal/gb"
)

// frameRGBA writes the last frame of the console into dst as RGBA pixels.
// DMG shades go through pal; Game Boy Color frames are RGB555, optionally
// adjusted to look like the original screen (see correctColor).
func frameRGBA(dst []byte, console *gb.GameBoy, pal *Palette, correct bool) {
	if !console.IsCGB() {
		for i, s := range console.Framebuffer() {
			c := pal.Colors[s]
			dst[i*4], dst[i*4+1], dst[i*4+2], dst[i*4+3] = c.R, c.G, c.B, 0xFF
		}
		return
	}
	for i, c := range console.ColorFramebuffer() {
		r, g, b := rgb555(c, correct)
		dst[i*4], dst[i*4+1], dst[i*4+2], dst[i*4+3] = r, g, b, 0xFF
	}
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
