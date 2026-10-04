package ui

import (
	"slices"
	"strings"
	"testing"
)

func TestWrapText(t *testing.T) {
	for _, tc := range []struct {
		s     string
		chars int
		want  []string
	}{
		{"P: palette   F11: plein ecran", 40, []string{"P: palette   F11: plein ecran"}},
		{"P: palette   F11: plein ecran", 20, []string{"P: palette", "F11: plein ecran"}},
		{"P: palette  F11: plein ecran  Start+Select: menu", 30, []string{"P: palette  F11: plein ecran", "Start+Select: menu"}},
		{"Fichier non supporte: notes.txt (.gb, .gbc ou .zip)", 20, []string{"Fichier non", "supporte: notes.txt", "(.gb, .gbc ou .zip)"}},
		{"Sauvegarde du 04/10\nPartie DMG", 40, []string{"Sauvegarde du 04/10", "Partie DMG"}},
		{"Supercalifragilistic", 10, []string{"Superca..."}},
	} {
		if got := wrapText(tc.s, float64(tc.chars)*advance("a")); !slices.Equal(got, tc.want) {
			t.Errorf("wrapText(%q, %d chars) = %q, want %q", tc.s, tc.chars, got, tc.want)
		}
	}
}

func TestShorten(t *testing.T) {
	w := advance("a")
	if got := shorten("Volume    < 80% >", 14*w); got != "Volume < 80% >" {
		t.Errorf("aligned entry: %q", got)
	}
	if got := shorten("Palette   < DMG vert >", 14*w); got != "Palette < D..." {
		t.Errorf("long entry: %q", got)
	}
	if got := shorten("Quitter", 14*w); got != "Quitter" {
		t.Errorf("short entry: %q", got)
	}
}

// Every page of the menu fits the smallest windows (x1 scale, with and
// without HiDPI), its selected entry shown.
func TestMenuFitsSmallWindows(t *testing.T) {
	g := withGameBoy(t, newTestGame(t, newFakePads()))
	g.cfg.Recent = []RecentGame{{Path: "/r/a.gb", Title: "A"}, {Path: "/r/Pokemon - Version Jaune (France).zip"}}
	views := map[string]menu{
		"main":     {open: true, page: pageMain, cursor: itemQuit},
		"display":  {open: true, page: pageDisplay, cursor: displayBack},
		"controls": {open: true, page: pageControls, cursor: controlsBack()},
		"pad":      {open: true, page: pageControls, tab: tabPad},
		"recent":   {open: true, page: pageRecent, cursor: 2},
		"start":    {open: true, page: pageStart},
	}
	for _, size := range [][2]float64{{160, 144}, {320, 288}} {
		for name, m := range views {
			for _, lang := range []string{"en", "fr"} {
				g.cfg.Language = lang
				v := m.view(g)
				l := layoutMenu(v, 0, size[0], size[1])
				border := 2 * l.scale
				if l.x < border || l.y < border || l.x+l.w > size[0]-border || l.y+l.h > size[1]-border {
					t.Errorf("%s (%s) at %vx%v: panel %v,%v %vx%v off screen", name, lang, size[0], size[1], l.x, l.y, l.w, l.h)
				}
				if l.selected < 0 || l.first+l.selected != v.selected {
					t.Errorf("%s (%s) at %vx%v: selection hidden", name, lang, size[0], size[1])
				}
				lines := slices.Clone(l.footer)
				for _, s := range l.items {
					lines = append(lines, "> "+s)
				}
				for _, s := range lines {
					if advance(s)*l.scale > l.w-2*l.pad {
						t.Errorf("%s (%s) at %vx%v: %q wider than the panel", name, lang, size[0], size[1], s)
					}
				}
			}
		}
	}
}

// At full size, nothing changes: no scrolling, nothing shortened.
func TestMenuFullSize(t *testing.T) {
	g := withGameBoy(t, newTestGame(t, newFakePads()))
	m := menu{open: true, page: pageControls}
	v := m.view(g)
	l := layoutMenu(v, 0, 640, 576)
	if l.lineH != 16 || l.pad != 10 || l.above || l.below || !slices.Equal(l.items, v.items) {
		t.Errorf("layout at x4: line %v, pad %v, items %d of %d", l.lineH, l.pad, len(l.items), len(v.items))
	}
	if got := strings.Join(l.footer, "|"); got != v.footer {
		t.Errorf("footer %q, want %q", got, v.footer)
	}
}

// Scrolled, the entries keep the selected one in the middle, and say what
// is hidden.
func TestMenuScroll(t *testing.T) {
	v := menuView{title: "T"}
	for range 20 {
		v.items = append(v.items, "entry")
	}
	for _, sel := range []int{0, 10, 19} {
		v.selected = sel
		l := layoutMenu(v, 0, 160, 144)
		if l.first+l.selected != sel || l.above != (l.first > 0) || l.below != (l.first+len(l.items) < 20) {
			t.Errorf("selected %d: first %d, shown %d, above %v, below %v", sel, l.first, len(l.items), l.above, l.below)
		}
	}
}
