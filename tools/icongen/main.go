// Command icongen draws the icon of the application, and writes it in the
// formats each system wants: PNG at several sizes, gbe.icns for macOS and
// gbe.ico for Windows.
//
// The icon is a Game Boy in pixel art on a rounded tile, as macOS draws its
// icons. It is computed with integers only, so that every machine writes the
// same bytes: the generated files are in the repository, and a test checks
// that they match this program.
//
//	go run ./tools/icongen -out assets/icon
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

// body is the Game Boy, 22x32 cells: b the case, k the frame of the screen,
// S the screen, L the power light, n the logo, x the cross, a the A and B
// buttons, s Start and Select, p the speaker slots, . nothing.
var body = []string{
	"bbbbbbbbbbbbbbbbbbbbbb",
	"bbbbbbbbbbbbbbbbbbbbbb",
	"bbkkkkkkkkkkkkkkkkkkbb",
	"bbkkkkkkkkkkkkkkkkkkbb",
	"bbkkkSSSSSSSSSSSSkkkbb",
	"bbkkkSSSSSSSSSSSSkkkbb",
	"bbkkkSSSSSSSSSSSSkkkbb",
	"bbkkkSSSSSSSSSSSSkkkbb",
	"bbkLkSSSSSSSSSSSSkkkbb",
	"bbkkkSSSSSSSSSSSSkkkbb",
	"bbkkkSSSSSSSSSSSSkkkbb",
	"bbkkkSSSSSSSSSSSSkkkbb",
	"bbkkkSSSSSSSSSSSSkkkbb",
	"bbkkkSSSSSSSSSSSSkkkbb",
	"bbkkkkkkkkkkkkkkkkkkbb",
	"bbkkkkkkkkkkkkkkkkkkbb",
	"bbkkkkkkkkkkkkkkkkkbbb",
	"bbbbbbbbbbbbbbbbbbbbbb",
	"bbnnnnnnnbbbbbbbbbbbbb",
	"bbbbbbbbbbbbbbbbbbbbbb",
	"bbbbxxbbbbbbbbbbbbbbbb",
	"bbbbxxbbbbbbbbbbbaabbb",
	"bbxxxxxxbbbbbbbbbaabbb",
	"bbxxxxxxbbbbbbaabbbbbb",
	"bbbbxxbbbbbbbbaabbbbbb",
	"bbbbxxbbbbbbbbbbbbbbbb",
	"bbbbbbbbbbbbbbbbbbbbbb",
	"bbbbbbbbbbbbbbbbpbbpbb",
	"bbbbbbbssbbssbbpbbpbbb",
	"bbbbbbbbbbbbbbpbbpbbb.",
	"bbbbbbbbbbbbbbbbbbbb..",
	".bbbbbbbbbbbbbbbbbb...",
}

// screen is what the screen shows, 12x10 cells in its four shades of green,
// from 0 (the lightest) to 3: clouds, a hill and the ground.
var screen = []string{
	"000000000000",
	"000000000110",
	"001100001111",
	"011110000000",
	"000000000000",
	"000000003000",
	"000100033300",
	"001110333330",
	"222222222222",
	"222222222222",
}

func rgb(v uint32) color.RGBA { return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255} }

var (
	lcd     = [4]color.RGBA{rgb(0x9bbc0f), rgb(0x8bac0f), rgb(0x306230), rgb(0x0f380f)}
	palette = map[byte]color.RGBA{
		'b': rgb(0xcdc9c0),
		'k': rgb(0x5f6372),
		'L': rgb(0xe3262e),
		'n': rgb(0x2c2f7a),
		'x': rgb(0x26262c),
		'a': rgb(0xa3195b),
		's': rgb(0x8d8d99),
		'p': rgb(0x8f8b83),
	}
	shade = rgb(0xa7a39a) // the case along its right and bottom edges
	// The tile goes from top to bottom, from the magenta of the buttons to a
	// darker one.
	tileTop, tileBottom = rgb(0xb3266b), rgb(0x6e0f3e)
)

const (
	size   = 1024 // the drawing; the other sizes are reduced from it
	tile   = 824  // the tile and its corner radius, on Apple's icon grid
	radius = 185
	cell   = 19 // the side of a cell of the Game Boy
)

// Sizes are the sizes of the PNG files, in pixels.
var Sizes = []int{16, 32, 48, 64, 128, 256, 512, 1024}

func main() {
	out := flag.String("out", "assets/icon", "directory to write the files to")
	flag.Parse()
	files, err := Files()
	if err != nil {
		log.Fatal(err)
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(*out, name), b, 0o644); err != nil {
			log.Fatal(err)
		}
	}
}

// Files returns the content of each file, by name.
func Files() (map[string][]byte, error) {
	big := draw()
	pngs := map[int][]byte{}
	files := map[string][]byte{}
	for _, n := range Sizes {
		img := big
		if n != size {
			img = shrink(big, n)
		}
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			return nil, err
		}
		pngs[n] = b.Bytes()
		files[fmt.Sprintf("icon-%d.png", n)] = b.Bytes()
	}
	files["gbe.icns"] = icns(pngs)
	files["gbe.ico"] = ico(pngs)
	return files, nil
}

// draw draws the icon at size x size.
func draw() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	o := (size - tile) / 2
	for y := range tile {
		top := lerp(tileTop, tileBottom, y, tile)
		for x := range tile {
			// The corners are smoothed: the part of the pixel inside the
			// tile, out of 4x4 points.
			cov := 0
			for sy := range 4 {
				for sx := range 4 {
					if inTile(8*x+2*sx+1, 8*y+2*sy+1) {
						cov++
					}
				}
			}
			if cov > 0 {
				img.SetRGBA(o+x, o+y, scale(top, cov, 16))
			}
		}
	}
	at := func(x, y int) byte {
		if y < 0 || y >= len(body) || x < 0 || x >= len(body[y]) {
			return '.'
		}
		return body[y][x]
	}
	ox, oy := (size-22*cell)/2, (size-32*cell)/2
	// The shadow of the Game Boy on the tile, one cell lower right.
	for y := range body {
		for x := range body[y] {
			if at(x, y) != '.' {
				square(img, ox+(x+1)*cell, oy+(y+1)*cell, func(p color.RGBA) color.RGBA {
					a := p.A
					p = scale(p, 185, 255)
					p.A = a
					return p
				})
			}
		}
	}
	for y := range body {
		for x := range body[y] {
			c, ok := palette[at(x, y)]
			switch at(x, y) {
			case '.':
				continue
			case 'b':
				if at(x+1, y) == '.' || at(x, y+1) == '.' {
					c = shade
				}
			case 'S':
				c, ok = lcd[screen[y-4][x-5]-'0'], true
			}
			if !ok {
				panic(fmt.Sprintf("no color for %q", at(x, y)))
			}
			square(img, ox+x*cell, oy+y*cell, func(color.RGBA) color.RGBA { return c })
		}
	}
	return img
}

// inTile reports whether the point (x/8, y/8) of the tile is inside its
// rounded corners.
func inTile(x, y int) bool {
	const t, r = 8 * tile, 8 * radius
	cx, cy := min(max(x, r), t-r), min(max(y, r), t-r)
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r
}

// square sets the cell at (x0, y0) to f of each of its pixels, where the
// image is not empty.
func square(img *image.RGBA, x0, y0 int, f func(color.RGBA) color.RGBA) {
	for y := y0; y < y0+cell; y++ {
		for x := x0; x < x0+cell; x++ {
			if p := img.RGBAAt(x, y); p.A != 0 {
				img.SetRGBA(x, y, f(p))
			}
		}
	}
}

// scale multiplies the color, alpha premultiplied, by num/den.
func scale(c color.RGBA, num, den int) color.RGBA {
	f := func(v uint8) uint8 { return uint8((int(v)*num + den/2) / den) }
	return color.RGBA{f(c.R), f(c.G), f(c.B), f(c.A)}
}

// lerp goes from a to b as i goes from 0 to n.
func lerp(a, b color.RGBA, i, n int) color.RGBA {
	f := func(a, b uint8) uint8 { return uint8(int(a) + ((int(b)-int(a))*i*2+n)/(2*n)) }
	return color.RGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 255}
}

// shrink reduces img to n x n, each pixel the mean of the part of img it
// covers, weighted by how much of each pixel it covers.
func shrink(img *image.RGBA, n int) *image.RGBA {
	// In units of 1/n of a pixel of img, its pixel i covers [i*n, (i+1)*n)
	// and the pixel x of the result covers [x*size, (x+1)*size).
	weights := func(x int) (first int, w []int) {
		lo, hi := x*size, (x+1)*size
		first = lo / n
		for i := first; i*n < hi; i++ {
			w = append(w, min(hi, (i+1)*n)-max(lo, i*n))
		}
		return first, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		y0, wy := weights(y)
		for x := range n {
			x0, wx := weights(x)
			var sum [4]int
			for j, a := range wy {
				for i, b := range wx {
					p := img.RGBAAt(x0+i, y0+j)
					sum[0] += a * b * int(p.R)
					sum[1] += a * b * int(p.G)
					sum[2] += a * b * int(p.B)
					sum[3] += a * b * int(p.A)
				}
			}
			const k = size * size
			avg := func(s int) uint8 { return uint8((s + k/2) / k) }
			dst.SetRGBA(x, y, color.RGBA{avg(sum[0]), avg(sum[1]), avg(sum[2]), avg(sum[3])})
		}
	}
	return dst
}

// icns is the macOS icon file: a list of PNG images, each with its type,
// as iconutil writes them from an .iconset.
func icns(pngs map[int][]byte) []byte {
	entries := []struct {
		kind string
		size int
	}{
		{"icp4", 16}, {"icp5", 32}, {"ic11", 32}, {"ic12", 64},
		{"ic07", 128}, {"ic08", 256}, {"ic13", 256}, {"ic09", 512}, {"ic14", 512}, {"ic10", 1024},
	}
	var body bytes.Buffer
	for _, e := range entries {
		body.WriteString(e.kind)
		binary.Write(&body, binary.BigEndian, uint32(8+len(pngs[e.size])))
		body.Write(pngs[e.size])
	}
	var b bytes.Buffer
	b.WriteString("icns")
	binary.Write(&b, binary.BigEndian, uint32(8+body.Len()))
	b.Write(body.Bytes())
	return b.Bytes()
}

// ico is the Windows icon file: a directory of PNG images, up to 256 px.
func ico(pngs map[int][]byte) []byte {
	sizes := []int{16, 32, 48, 64, 128, 256}
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))}) // reserved, type icon, count
	offset := 6 + 16*len(sizes)
	for _, n := range sizes {
		side := uint8(n % 256) // 0 stands for 256
		binary.Write(&b, binary.LittleEndian, struct {
			W, H, Colors, Reserved uint8
			Planes, Bits           uint16
			Len, Offset            uint32
		}{side, side, 0, 0, 1, 32, uint32(len(pngs[n])), uint32(offset)})
		offset += len(pngs[n])
	}
	for _, n := range sizes {
		b.Write(pngs[n])
	}
	return b.Bytes()
}
