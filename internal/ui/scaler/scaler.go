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

// Frame is the picture to draw.
type Frame struct {
	Image *ebiten.Image // the LCD, at the resolution of the Game Boy
	// Changed is set when Image changed since the last Draw: the passes
	// run again.
	Changed bool
	// Ghost blends each frame with the previous one, like the slow LCD of
	// the console did: games relied on it to make flickering sprites look
	// transparent. Advance is set when the console produced a new frame
	// since the last Draw.
	Ghost, Advance bool
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
	images  []*ebiten.Image // output of each pass
	filter  string          // filter whose output images holds

	ghost    [2]*ebiten.Image // the blended frame, and the previous frame
	ghosting bool             // ghost holds the previous frame
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
	src := fr.Image
	changed := fr.Changed || p.filter != f.ID
	if fr.Ghost {
		src = p.blend(src, fr.Advance)
		changed = changed || fr.Advance
	} else {
		p.ghosting = false
	}
	if changed {
		src = p.run(f, src)
	} else if len(f.Passes) > 0 {
		src = p.images[len(f.Passes)-1]
	}

	sw, sh := float64(dst.Bounds().Dx()), float64(dst.Bounds().Dy())
	lw, lh := float64(fr.Image.Bounds().Dx()), float64(fr.Image.Bounds().Dy())
	x, y, scale := Placement(sw, sh, lw, lh, f.Final == "")
	if f.Final == "" {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(x, y)
		dst.DrawImage(src, op)
		return
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	sx, sy := math.Round(lw*scale)/float64(w), math.Round(lh*scale)/float64(h)
	op := &ebiten.DrawRectShaderOptions{}
	op.GeoM.Scale(sx, sy)
	op.GeoM.Translate(x, y)
	op.Images[0] = src
	gap := func(v uint8) float32 { return float32(v) / 0xFF }
	color := float32(0)
	if fr.Color {
		color = 1
	}
	op.Uniforms = map[string]any{
		"Scale": []float32{float32(sx), float32(sy)},
		"Color": color,
		"Gap":   []float32{gap(fr.Gap.R), gap(fr.Gap.G), gap(fr.Gap.B)},
	}
	dst.DrawRectShader(w, h, p.shader(f.Final), op)
}

// run runs the passes of f on src, and returns the last output.
func (p *Pipeline) run(f *Filter, src *ebiten.Image) *ebiten.Image {
	if p.filter != f.ID {
		for _, img := range p.images {
			img.Deallocate()
		}
		p.images = make([]*ebiten.Image, len(f.Passes))
		p.filter = f.ID
	}
	for i, pass := range f.Passes {
		w, h := src.Bounds().Dx(), src.Bounds().Dy()
		out := sized(&p.images[i], w*pass.Factor, h*pass.Factor)
		op := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy}
		op.GeoM.Scale(float64(pass.Factor), float64(pass.Factor))
		op.Images[0] = src
		out.DrawRectShader(w, h, p.shader(pass.Shader), op)
		src = out
	}
	return src
}

// blend returns src blended with the previous frame, which advance
// replaces with src.
func (p *Pipeline) blend(src *ebiten.Image, advance bool) *ebiten.Image {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	out, prev := sized(&p.ghost[0], w, h), sized(&p.ghost[1], w, h)
	if !p.ghosting { // nothing to blend with yet
		prev.DrawImage(src, &ebiten.DrawImageOptions{Blend: ebiten.BlendCopy})
		out.DrawImage(src, &ebiten.DrawImageOptions{Blend: ebiten.BlendCopy})
		p.ghosting = true
		return out
	}
	if advance {
		op := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy}
		op.Images[0], op.Images[1] = src, prev
		out.DrawRectShader(w, h, p.shader("ghost"), op)
		prev.DrawImage(src, &ebiten.DrawImageOptions{Blend: ebiten.BlendCopy})
	}
	return out
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
	names := []string{"ghost"}
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
