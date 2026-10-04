// Package scaler draws the 160x144 picture of the Game Boy on screens of any
// size, with filters made for its low resolution.
//
// A filter is a chain of GPU passes (Kage shaders in shaders/): passes that
// enlarge the picture by a fixed factor, such as MMPX, then a final pass that
// draws it to the screen at any scale. The fixed passes only run when the
// picture changes, and their cost does not depend on the screen size: only
// the final pass, a cheap one, does.
//
// Adding a filter takes a shader in shaders/, an entry in Filters and its
// name in the languages of internal/i18n (filter.<id>).
package scaler

import (
	"embed"
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// Pass is a shader run on the output of the previous pass (first, on the
// picture of the Game Boy).
type Pass struct {
	Shader string // file name in shaders/, without .kage
	Factor int    // output size, in input pixels
}

// Filter is a way of drawing the picture.
type Filter struct {
	ID     string // saved in the config; the name to show is filter.<ID>
	Passes []Pass
	// Final is the shader that draws the last image to the screen, scaled
	// to fill it. "" draws it as is, at the largest integer scale.
	Final string
}

// Filters lists the available filters, the default one first.
var Filters = []Filter{
	{ID: "nearest"},
	{ID: "sharp", Final: "sharp"},
	{ID: "lcd", Final: "lcd"},
	{ID: "scale2x", Passes: []Pass{{Shader: "scale2x", Factor: 2}}, Final: "sharp"},
	{ID: "scale3x", Passes: []Pass{{Shader: "scale3x", Factor: 3}}, Final: "sharp"},
	{ID: "mmpx", Passes: []Pass{{Shader: "mmpx", Factor: 2}}, Final: "sharp"},
}

// Index returns the position of filter id in Filters, or 0 (the default)
// if there is none.
func Index(id string) int {
	for i, f := range Filters {
		if f.ID == id {
			return i
		}
	}
	return 0
}

// IDs lists the filter IDs, in the order of Filters.
func IDs() []string {
	ids := make([]string, len(Filters))
	for i, f := range Filters {
		ids[i] = f.ID
	}
	return ids
}

//go:embed shaders/*.kage
var shaderFiles embed.FS

// commonFile holds the helpers every shader may use: it is inserted after
// the header of each one (Kage has no includes).
const commonFile = "common"

const header = "//kage:unit pixels\n\npackage main\n\n"

// source returns the full Kage source of shader name.
func source(name string) ([]byte, error) {
	common, err := shaderFiles.ReadFile("shaders/" + commonFile + ".kage")
	if err != nil {
		return nil, err
	}
	body, err := shaderFiles.ReadFile("shaders/" + name + ".kage")
	if err != nil {
		return nil, err
	}
	return []byte(header + string(common) + "\n" + string(body)), nil
}

// GhostMode is how a frame is blended with the previous one.
type GhostMode int

const (
	GhostOff GhostMode = iota
	// GhostSimple shows both frames equally.
	GhostSimple
	// GhostAccurate weighs them by 1/3 and 2/3, alternately from one line
	// to the next and from one frame to the next, as the lines of the LCD
	// fade unevenly. It is the accurate frame blending of SameBoy.
	GhostAccurate
)

// Frame is the picture to draw.
type Frame struct {
	Image *ebiten.Image // the LCD, at the resolution of the Game Boy
	// Changed is set when Image changed since the last Draw: the passes
	// run again.
	Changed bool
	// Ghost blends Image with Prev, the frame the console produced just
	// before it, like the slow LCD of the console did: games relied on it
	// to make flickering sprites look transparent. The frames are blended
	// in linear light, after the filter, which sees clean frames. Prev and
	// PrevChanged are like Image and Changed.
	Ghost       GhostMode
	Prev        *ebiten.Image
	PrevChanged bool
	// Odd is the parity of the frame, which alternates the weights of the
	// lines in GhostAccurate.
	Odd bool
	// Color is set for a Game Boy Color picture, which the LCD filter draws
	// with red, green and blue stripes.
	Color bool
	// Gap is the color seen between the dots of a DMG LCD: the lightest
	// shade of the palette.
	Gap color.RGBA
}

// Pipeline draws frames with a filter. It keeps the shaders and the
// intermediate images between frames. The zero value is ready to use.
type Pipeline struct {
	shaders map[string]*ebiten.Shader
	filter  string // filter whose passes cur and prev hold
	cur     passes // the passes of the current frame
	prev    passes // and those of the previous frame, when ghosting
}

// passes holds the output of each pass of a filter for one frame.
type passes struct {
	images []*ebiten.Image
	valid  bool // the images hold the passes of the last source
}

// reset drops the images, for a filter with n passes.
func (ps *passes) reset(n int) {
	for _, img := range ps.images {
		if img != nil {
			img.Deallocate()
		}
	}
	ps.images, ps.valid = make([]*ebiten.Image, n), false
}

// shader returns the compiled shader name. The shaders are embedded and
// tested, so a compilation error is a bug.
func (p *Pipeline) shader(name string) *ebiten.Shader {
	if s := p.shaders[name]; s != nil {
		return s
	}
	src, err := source(name)
	if err == nil {
		var s *ebiten.Shader
		if s, err = ebiten.NewShader(src); err == nil {
			if p.shaders == nil {
				p.shaders = map[string]*ebiten.Shader{}
			}
			p.shaders[name] = s
			return s
		}
	}
	panic(fmt.Sprintf("scaler: shader %s: %v", name, err))
}

// sized returns *img, allocated again if it does not have size w x h. The
// images are kept out of the texture atlas, which their size would waste.
func sized(img **ebiten.Image, w, h int) *ebiten.Image {
	if *img != nil && (*img).Bounds().Dx() == w && (*img).Bounds().Dy() == h {
		return *img
	}
	if *img != nil {
		(*img).Deallocate()
	}
	*img = ebiten.NewImageWithOptions(image.Rect(0, 0, w, h), &ebiten.NewImageOptions{Unmanaged: true})
	return *img
}

// Draw draws fr to dst, centered and scaled to fit, with filter f.
func (p *Pipeline) Draw(dst *ebiten.Image, f *Filter, fr Frame) {
	if p.filter != f.ID {
		p.cur.reset(len(f.Passes))
		p.prev.reset(len(f.Passes))
		p.filter = f.ID
	}
	src := p.run(&p.cur, f, fr.Image, fr.Changed)
	var prev *ebiten.Image
	if fr.Ghost != GhostOff && fr.Prev != nil {
		prev = p.run(&p.prev, f, fr.Prev, fr.PrevChanged)
	}

	sw, sh := float64(dst.Bounds().Dx()), float64(dst.Bounds().Dy())
	lw, lh := float64(fr.Image.Bounds().Dx()), float64(fr.Image.Bounds().Dy())
	x, y, scale := Placement(sw, sh, lw, lh, f.Final == "")
	final := f.Final
	if final == "" {
		final = "nearest"
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	sx, sy := math.Round(lw*scale)/float64(w), math.Round(lh*scale)/float64(h)
	op := &ebiten.DrawRectShaderOptions{}
	op.GeoM.Scale(sx, sy)
	op.GeoM.Translate(x, y)
	op.Images[0] = src
	ghost := float32(0)
	if prev != nil {
		op.Images[1] = prev
		ghost = float32(fr.Ghost)
	}
	gap := func(v uint8) float32 { return float32(v) / 0xFF }
	op.Uniforms = map[string]any{
		"Scale":      []float32{float32(sx), float32(sy)},
		"Color":      flag(fr.Color),
		"Gap":        []float32{gap(fr.Gap.R), gap(fr.Gap.G), gap(fr.Gap.B)},
		"Ghost":      ghost,
		"Odd":        flag(fr.Odd),
		"LineHeight": float32(float64(h) / lh),
	}
	dst.DrawRectShader(w, h, p.shader(final), op)
}

func flag(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// run runs the passes of f on src, unless ps already holds them (the source
// did not change), and returns the last output, or src without passes.
func (p *Pipeline) run(ps *passes, f *Filter, src *ebiten.Image, changed bool) *ebiten.Image {
	if len(f.Passes) == 0 {
		return src
	}
	if ps.valid && !changed {
		return ps.images[len(ps.images)-1]
	}
	for i, pass := range f.Passes {
		w, h := src.Bounds().Dx(), src.Bounds().Dy()
		out := sized(&ps.images[i], w*pass.Factor, h*pass.Factor)
		op := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy}
		op.GeoM.Scale(float64(pass.Factor), float64(pass.Factor))
		op.Images[0] = src
		out.DrawRectShader(w, h, p.shader(pass.Shader), op)
		src = out
	}
	ps.valid = true
	return src
}

// Placement returns where to draw a w x h picture on a sw x sh screen:
// its top-left corner and its scale. The picture is centered and as large
// as fits, at an integer scale when integer is set (except on screens
// smaller than the picture). The corner is on a pixel boundary.
func Placement(sw, sh, w, h float64, integer bool) (x, y, scale float64) {
	scale = math.Min(sw/w, sh/h)
	if integer && scale >= 1 {
		scale = math.Floor(scale)
	}
	return math.Floor((sw - math.Round(w*scale)) / 2), math.Floor((sh - math.Round(h*scale)) / 2), scale
}

// shaderNames lists the shaders of the filters, for the tests.
func shaderNames() []string {
	names := []string{"nearest"}
	for _, f := range Filters {
		for _, p := range f.Passes {
			names = append(names, p.Shader)
		}
		if f.Final != "" {
			names = append(names, f.Final)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// ID returns the filter ID matching s case-insensitively, for the command
// line.
func ID(s string) (string, bool) {
	for _, f := range Filters {
		if strings.EqualFold(f.ID, s) {
			return f.ID, true
		}
	}
	return "", false
}
