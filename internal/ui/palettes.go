package ui

import (
	"image/color"
	"strings"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/i18n"
)

// Palette maps the four DMG shades (lightest first) to screen colors.
type Palette struct {
	ID     string
	Name   string
	Colors [4]color.RGBA
}

func rgb(v uint32) color.RGBA {
	return color.RGBA{byte(v >> 16), byte(v >> 8), byte(v), 0xFF}
}

func pal(id, name string, c0, c1, c2, c3 uint32) Palette {
	return Palette{id, name, [4]color.RGBA{rgb(c0), rgb(c1), rgb(c2), rgb(c3)}}
}

// Palettes lists the built-in color schemes. The first one is the default.
var Palettes = []Palette{
	pal("dmg", "DMG vert", 0x9BBC0F, 0x8BAC0F, 0x306230, 0x0F380F),
	pal("bgb", "BGB", 0xE0F8D0, 0x88C070, 0x346856, 0x081820),
	pal("pocket", "Pocket", 0xC4CFA1, 0x8B956D, 0x4D533C, 0x1F1F1F),
	pal("light", "Light (turquoise)", 0x7BE7D1, 0x3FB5A3, 0x1F6F6B, 0x0A3533),
	pal("grey", "Noir et blanc", 0xFFFFFF, 0xAAAAAA, 0x555555, 0x000000),
	pal("amber", "Ambre", 0xFFD37F, 0xD9912B, 0x8C4A10, 0x3A1A05),
	pal("ice", "Bleu glacier", 0xE0F8FF, 0x86C0E8, 0x3A6EA5, 0x0E1F4A),
	pal("vboy", "Rouge Virtual Boy", 0xEF0000, 0xA40000, 0x550000, 0x000000),
	pal("sepia", "Sepia", 0xF4E4C1, 0xC2A477, 0x7A5C3A, 0x2E2014),
	pal("purple", "Violet", 0xF2D5F7, 0xC07FD1, 0x6E3A8C, 0x2A0F3A),
}

// The palettes of colorized DMG games that the CGB boot ROM offers through
// the buttons held during its logo, in the order of gb.SetCompatPalette: the
// four directions, alone, with A, then with B.
var compatDirections = [...]gb.Button{gb.ButtonRight, gb.ButtonLeft, gb.ButtonUp, gb.ButtonDown}

func compatButtons(i int) (dir gb.Button, button string) {
	return compatDirections[i%4], [...]string{"", "A", "B"}[i/4]
}

// compatPaletteID names palette i in the config, e.g. "left+b".
func compatPaletteID(i int) string {
	dir, button := compatButtons(i)
	id := strings.ToLower(dir.String())
	if button != "" {
		id += "+" + strings.ToLower(button)
	}
	return id
}

// compatPaletteIndex is the palette named id, or gb.CompatAuto.
func compatPaletteIndex(id string) int {
	for i := range gb.CompatKeyPalettes {
		if compatPaletteID(i) == id {
			return i
		}
	}
	return gb.CompatAuto
}

// compatPaletteLabel names palette i (or gb.CompatAuto) in language l, by
// its buttons: "Left+B".
func compatPaletteLabel(l *i18n.Locale, i int) string {
	if i == gb.CompatAuto {
		return l.T("colors.auto")
	}
	dir, button := compatButtons(i)
	label := bindingLabel(l, dir.String())
	if button != "" {
		label += "+" + button
	}
	return label
}

func paletteIndex(id string) int {
	for i, p := range Palettes {
		if p.ID == id {
			return i
		}
	}
	return 0
}
