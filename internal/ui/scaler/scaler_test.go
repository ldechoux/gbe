package scaler

import (
	"slices"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The shaders are compiled at run time: a syntax or type error would only
// show when the filter is chosen.
func TestShadersCompile(t *testing.T) {
	for _, name := range shaderNames() {
		src, err := source(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if _, err := ebiten.NewShader(src); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestFilters(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range Filters {
		if seen[f.ID] {
			t.Errorf("filter %q listed twice", f.ID)
		}
		seen[f.ID] = true
		for _, p := range f.Passes {
			if p.Factor < 1 {
				t.Errorf("%s: pass %s has factor %d", f.ID, p.Shader, p.Factor)
			}
		}
		if len(f.Passes) > 0 && f.Final == "" {
			t.Errorf("%s: passes need a final shader", f.ID)
		}
	}
	if Index("mmpx") != slices.IndexFunc(Filters, func(f Filter) bool { return f.ID == "mmpx" }) || Index("nope") != 0 {
		t.Error("Index")
	}
	if id, ok := ID("MMPX"); !ok || id != "mmpx" {
		t.Errorf("ID(MMPX) = %q, %v", id, ok)
	}
	if _, ok := ID("nope"); ok {
		t.Error("ID(nope) found")
	}
}

func TestPlacement(t *testing.T) {
	for _, c := range []struct {
		name       string
		sw, sh     float64
		integer    bool
		x, y, w, h float64
	}{
		{"1080p integer", 1920, 1080, true, 400, 36, 1120, 1008},
		{"1080p fit", 1920, 1080, false, 360, 0, 1200, 1080},
		{"1440p fit", 2560, 1440, false, 480, 0, 1600, 1440},
		{"4K integer", 3840, 2160, true, 720, 0, 2400, 2160},
		{"4K fit", 3840, 2160, false, 720, 0, 2400, 2160},
		{"window x4", 640, 576, false, 0, 0, 640, 576},
		{"tall window", 640, 1000, false, 0, 212, 640, 576},
		{"too small", 80, 72, true, 0, 0, 80, 72},
	} {
		x, y, s := Placement(c.sw, c.sh, 160, 144, c.integer)
		if x != c.x || y != c.y || 160*s != c.w || 144*s != c.h {
			t.Errorf("%s: (%v, %v) %vx%v, want (%v, %v) %vx%v", c.name, x, y, 160*s, 144*s, c.x, c.y, c.w, c.h)
		}
	}
}
