package ui

import (
	"math"
	"regexp"
	"strings"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// The menu font is a 7x13 bitmap: it cannot shrink and stay readable. In a
// small window (x1 scale: 160x144, or 320x288 on HiDPI screens), the menu
// gets compact instead: tighter lines, its footer wrapped, long entries
// shortened, and the entries scrolled around the selected one.

// menuLayout is where draw puts a menu view on a screen.
type menuLayout struct {
	scale      float64 // of the text
	lineH, pad float64
	x, y, w, h float64 // the panel
	width      float64 // of its content, before scaling
	items      []string
	first      int  // index of items[0] in the view
	selected   int  // in items, -1 if hidden
	above      bool // entries hidden above items, and below
	below      bool
	footer     []string
}

// advance is the width of s in the menu font, before scaling.
func advance(s string) float64 { return text.Advance(s, menuFace) }

// shorten fits s in width: first without the spaces that align it
// ("Volume    < 80% >"), then cut, with "...".
func shorten(s string, width float64) string {
	if advance(s) <= width {
		return s
	}
	s = strings.Join(strings.Fields(s), " ")
	if advance(s) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && advance(string(r)+"...") > width {
		r = r[:len(r)-1]
	}
	return string(r) + "..."
}

// wrapText breaks s into lines of at most width. It breaks first between
// the parts of a line set apart by several spaces ("P: palette   F11:
// plein ecran"), then between words; a word too long on its own is
// shortened.
func wrapText(s string, width float64) []string {
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		if advance(para) <= width {
			lines = append(lines, para)
			continue
		}
		var parts []string
		for _, part := range wideSpaces.Split(strings.TrimSpace(para), -1) {
			parts = append(parts, pack(strings.Fields(part), " ", width)...)
		}
		lines = append(lines, pack(parts, "  ", width)...)
	}
	return lines
}

var wideSpaces = regexp.MustCompile(` {2,}`)

// pack puts the words on lines of at most width, sep apart.
func pack(words []string, sep string, width float64) []string {
	var lines []string
	line := ""
	for _, w := range words {
		switch {
		case line == "":
			line = w
		case advance(line+sep+w) <= width:
			line += sep + w
		default:
			lines = append(lines, shorten(line, width))
			line = w
		}
	}
	return append(lines, shorten(line, width))
}

// layoutMenu fits view in a sw x sh screen. minWidth keeps the panel from
// shrinking while its text changes (menu.width).
func layoutMenu(v menuView, minWidth, sw, sh float64) menuLayout {
	l := menuLayout{scale: math.Max(1, math.Floor(sh/300))}
	l.lineH, l.pad = 16*l.scale, 10*l.scale
	border := 2 * l.scale
	tabsW := 0.0
	for _, t := range v.tabs {
		tabsW += advance(" "+t.label+" ") + 2*advance(" ")
	}

	fit := func(compact bool) {
		if compact {
			l.lineH, l.pad = 14*l.scale, 4*l.scale
		}
		maxW := (sw - 2*l.pad - 2*border) / l.scale // content, before scaling
		// An empty footer still leaves a blank line at full size.
		l.footer = nil
		if v.footer != "" || !compact {
			l.footer = wrapText(v.footer, maxW)
		}
		l.items = make([]string, len(v.items))
		for i, s := range v.items {
			l.items[i] = shorten(s, maxW-advance("> "))
		}
		width := math.Max(advance("  "+v.title), tabsW)
		for _, s := range l.items {
			width = math.Max(width, advance("> "+s))
		}
		for _, s := range v.altItems {
			width = math.Max(width, advance("> "+shorten(s, maxW-advance("> "))))
		}
		lines := len(l.footer)
		for _, f := range v.altFooters {
			alt := wrapText(f, maxW)
			lines = max(lines, len(alt))
			for _, s := range alt {
				width = math.Max(width, advance(s))
			}
		}
		for len(l.footer) < lines { // room for the longest footer
			l.footer = append(l.footer, "")
		}
		for _, s := range l.footer {
			width = math.Max(width, advance(s))
		}
		l.width = math.Min(math.Max(width, minWidth), maxW)
	}
	// The height of the panel, with n entries: the title, the tabs, the
	// entries and the footer (half a line apart).
	height := func(n int) float64 {
		h := float64(n)*l.lineH + 1.5*l.lineH + 2*l.pad
		if len(v.tabs) > 0 {
			h += 1.5 * l.lineH
		}
		if len(l.footer) > 0 {
			h += (float64(len(l.footer)) + 0.5) * l.lineH
		}
		return h
	}

	avail := sh - 2*border
	fit(false)
	if height(len(l.items)) > avail || l.width < math.Max(width0(v, tabsW), minWidth) {
		fit(true)
	}
	n := len(l.items)
	if height(n) > avail {
		// Scroll: as many entries as fit, at least 3, before the footer.
		fixed := height(0)
		if (avail-fixed)/l.lineH < 3 {
			l.footer = nil
			fixed = height(0)
		}
		n = max(1, min(n, int((avail-fixed)/l.lineH)))
	}
	l.selected = v.selected
	if n < len(l.items) {
		l.first = min(max(0, v.selected-n/2), len(l.items)-n)
		l.above, l.below = l.first > 0, l.first+n < len(l.items)
		l.items = l.items[l.first : l.first+n]
		l.selected = v.selected - l.first
	}
	if l.selected < 0 || l.selected >= len(l.items) {
		l.selected = -1
	}

	l.w = l.width*l.scale + 2*l.pad
	l.h = math.Min(height(len(l.items)), avail)
	l.x, l.y = math.Round((sw-l.w)/2), math.Round((sh-l.h)/2)
	return l
}

// width0 is the width the view needs, before scaling, unshortened.
func width0(v menuView, tabsW float64) float64 {
	width := math.Max(advance("  "+v.title), tabsW)
	for _, s := range v.items {
		width = math.Max(width, advance("> "+s))
	}
	for _, s := range v.altItems {
		width = math.Max(width, advance("> "+s))
	}
	for _, f := range append([]string{v.footer}, v.altFooters...) {
		for _, s := range strings.Split(f, "\n") {
			width = math.Max(width, advance(s))
		}
	}
	return width
}
