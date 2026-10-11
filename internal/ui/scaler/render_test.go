package scaler

import (
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/rendertest"
)

// The rendering tests draw frames of the benchmark ROM through every
// filter, and compare them with reference images (see internal/rendertest):
//
//	GBE_RENDER=1 go test ./internal/ui/scaler
func TestMain(m *testing.M) { rendertest.Main(m) }

// benchFrames runs the game scene of the benchmark ROM (benchrom/) on
// model, and returns its last two frames as images: the current one and
// the one before, for ghosting.
func benchFrames(t *testing.T, model gb.Model) (cur, prev *ebiten.Image) {
	t.Helper()
	data, err := os.ReadFile("../../../benchrom/gbe-bench.gbc")
	if err != nil {
		t.Fatal(err)
	}
	cart, err := gb.NewCartridge(data)
	if err != nil {
		t.Fatal(err)
	}
	g, err := gb.NewModel(cart, nil, model)
	if err != nil {
		t.Fatal(err)
	}
	for f := range 120 {
		g.SetButton(gb.ButtonA, f < 10) // the game scene
		g.RunFrame()
		if f == 118 {
			prev = frameImage(g)
		}
	}
	return frameImage(g), prev
}

// dmgShades are the 4 shades of a DMG frame: the green palette of gbe.
var dmgShades = [4]color.RGBA{{0x9b, 0xbc, 0x0f, 0xff}, {0x8b, 0xac, 0x0f, 0xff}, {0x30, 0x62, 0x30, 0xff}, {0x0f, 0x38, 0x0f, 0xff}}

// frameImage is the frame of the console as an image, its RGB555 colors
// widened without correction.
func frameImage(g *gb.GameBoy) *ebiten.Image {
	img := image.NewRGBA(image.Rect(0, 0, gb.ScreenWidth, gb.ScreenHeight))
	for i := range gb.ScreenWidth * gb.ScreenHeight {
		var c color.RGBA
		if g.IsCGB() {
			v := g.ColorFramebuffer()[i]
			c5 := func(s uint) uint8 { return uint8(uint32(v>>s&31) * 255 / 31) }
			c = color.RGBA{c5(0), c5(5), c5(10), 0xff}
		} else {
			c = dmgShades[g.Framebuffer()[i]&3]
		}
		img.SetRGBA(i%gb.ScreenWidth, i/gb.ScreenWidth, c)
	}
	return ebiten.NewImageFromImage(img)
}

// Every filter, on a DMG frame at x3.
func TestRenderFilters(t *testing.T) {
	rendertest.Skip(t)
	cur, _ := benchFrames(t, gb.ModelDMG)
	gap := color.RGBA{0x9b, 0xbc, 0x0f, 0xff}
	for _, f := range Filters {
		var p Pipeline
		dst := ebiten.NewImage(480, 432)
		p.Draw(dst, &f, Frame{Image: cur, Changed: true, Gap: gap})
		rendertest.Check(t, "dmg_"+f.ID, dst)
	}
}

// The filters that fill the screen, on a screen that is not a multiple of
// the Game Boy one (here 1.6 times, with black bars); nearest stays at a
// whole factor, centered.
func TestRenderFit(t *testing.T) {
	rendertest.Skip(t)
	cur, _ := benchFrames(t, gb.ModelDMG)
	for _, id := range []string{"nearest", "sharp", "lcd"} {
		var p Pipeline
		dst := ebiten.NewImage(384, 240)
		p.Draw(dst, &Filters[Index(id)], Frame{Image: cur, Changed: true, Gap: dmgShades[0]})
		rendertest.Check(t, "fit_"+id, dst)
	}
}

// A Game Boy Color frame under the LCD filter, whose grid follows the
// colors, and ghosting, simple and accurate, which blends the frame before.
func TestRenderColorAndGhosting(t *testing.T) {
	rendertest.Skip(t)
	cur, prev := benchFrames(t, gb.ModelCGB)
	var p Pipeline
	dst := ebiten.NewImage(480, 432)
	p.Draw(dst, &Filters[Index("lcd")], Frame{Image: cur, Changed: true, Color: true})
	rendertest.Check(t, "cgb_lcd", dst)

	for _, c := range []struct {
		name  string
		ghost GhostMode
		odd   bool
	}{
		{"ghost_simple", GhostSimple, false},
		{"ghost_accurate_even", GhostAccurate, false},
		{"ghost_accurate_odd", GhostAccurate, true},
	} {
		var p Pipeline
		dst := ebiten.NewImage(320, 288)
		p.Draw(dst, &Filters[Index("scale2x")], Frame{Image: cur, Changed: true, Ghost: c.ghost, Prev: prev, PrevChanged: true, Odd: c.odd, Color: true})
		rendertest.Check(t, c.name, dst)
	}

	// The passes of an unchanged frame are kept: drawn again, the same.
	again := ebiten.NewImage(320, 288)
	p.Draw(again, &Filters[Index("scale2x")], Frame{Image: cur, Changed: false, Color: true})
	p.Draw(again, &Filters[Index("scale2x")], Frame{Image: cur, Changed: false, Color: true})
	rendertest.Check(t, "cgb_scale2x", again)
}
