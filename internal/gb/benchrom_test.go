package gb

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"hash/fnv"
	"image"
	imgcolor "image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The benchmark ROM (benchrom/) is committed, so these tests always run. For
// each scene, on a DMG and on a Game Boy Color, they check the golden hash
// of the run (audio, every frame, final state) and compare its last frame
// with a reference image. On failure, the frame and the audio are written
// to $GBE_GOLDEN_OUT, when set, to see and hear what changed.
//
// After an intended change, -update rewrites the reference images and
// prints the new hashes:
//
//	go test ./internal/gb -run BenchROM -update

const benchROMPath = "../../benchrom/gbe-bench.gbc"

var update = flag.Bool("update", false, "rewrite the reference images of the benchmark ROM and print its golden hashes")

// benchScene is a scene of the benchmark ROM and the button that selects it
// at power on (none for the demo, which goes through every scene).
type benchScene struct {
	name   string
	button Button
	frames int
}

var benchScenes = []benchScene{
	{"game", ButtonA, 600},
	{"cpu", ButtonB, 600},
	{"idle", ButtonSelect, 600},
	{"color", ButtonStart, 600},
	{"sound", ButtonRight, 600},
	{"demo", -1, 5*256 + 200},
}

var benchModels = []struct {
	name  string
	model Model
}{{"dmg", ModelDMG}, {"cgb", ModelCGB}}

// newBenchROM powers on the benchmark ROM with the button of the scene
// held. The button is released by benchInputs.
func newBenchROM(tb testing.TB, s benchScene, model Model) *GameBoy {
	tb.Helper()
	rom, err := os.ReadFile(benchROMPath)
	if err != nil {
		tb.Fatal(err) // committed: never skipped
	}
	cart, err := NewCartridge(rom)
	if err != nil {
		tb.Fatal(err)
	}
	g, err := NewModel(cart, nil, model)
	if err != nil {
		tb.Fatal(err)
	}
	if s.button >= 0 {
		g.SetButton(s.button, true)
	}
	return g
}

// benchInputs releases the buttons after the first frames: the ROM only
// reads them at power on.
func benchInputs(g *GameBoy, frame int) {
	if frame == 10 {
		for _, b := range Buttons {
			g.SetButton(b, false)
		}
	}
}

func TestBenchROM(t *testing.T) {
	for _, s := range benchScenes {
		for _, m := range benchModels {
			name := "benchrom/" + s.name + "/" + m.name
			t.Run(s.name+"/"+m.name, func(t *testing.T) {
				g := newBenchROM(t, s, m.model)
				var audio []int16
				got := goldenRun(g, s.frames, benchInputs, func(samples []int16) {
					audio = append(audio, samples...)
				})
				img := frameImage(g)
				ref := filepath.Join("../../benchrom/ref", s.name+"-"+m.name+".png")
				if *update {
					writePNG(t, ref, img)
					fmt.Printf("\t%q: %#016x,\n", name, got)
					return
				}
				ok := checkReference(t, ref, img)
				if want, known := goldenHashes[name]; !known || got != want {
					t.Errorf("hash %#016x, want %#016x", got, want)
					ok = false
				}
				if !ok {
					dumpFailure(t, s.name+"-"+m.name, img, audio)
				}
			})
		}
	}
}

// frameImage returns the last frame: shades of grey on a DMG (as in the
// acid2 references), RGB555 colors widened to 8 bits on a CGB.
func frameImage(g *GameBoy) image.Image {
	r := image.Rect(0, 0, ScreenWidth, ScreenHeight)
	if !g.IsCGB() {
		img := image.NewGray(r)
		for i, s := range g.Framebuffer() {
			img.Pix[i] = 0xFF - s*0x55
		}
		return img
	}
	img := image.NewRGBA(r)
	for i, c := range g.ColorFramebuffer() {
		widen := func(v uint16) uint8 { v &= 0x1F; return uint8(v<<3 | v>>2) }
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = widen(c), widen(c>>5), widen(c>>10), 0xFF
	}
	return img
}

// checkReference compares img with the reference image at path.
func checkReference(t *testing.T, path string, img image.Image) bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Errorf("reference image: %v (run with -update to create it)", err)
		return false
	}
	defer f.Close()
	ref, err := png.Decode(f)
	if err != nil {
		t.Errorf("reference image: %v", err)
		return false
	}
	diff, first := 0, image.Point{}
	for y := range ScreenHeight {
		for x := range ScreenWidth {
			if !sameColor(ref.At(x, y), img.At(x, y)) {
				if diff == 0 {
					first = image.Pt(x, y)
				}
				diff++
			}
		}
	}
	if diff > 0 {
		t.Errorf("%d pixels differ from %s, the first at %v", diff, path, first)
		return false
	}
	return true
}

func sameColor(a, b imgcolor.Color) bool {
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// dumpFailure writes the last frame (PNG) and the audio (WAV) of a failed
// run to $GBE_GOLDEN_OUT.
func dumpFailure(t *testing.T, name string, img image.Image, audio []int16) {
	t.Helper()
	dir := os.Getenv("GBE_GOLDEN_OUT")
	if dir == "" {
		t.Log("set GBE_GOLDEN_OUT to a directory to get the frame and the audio of failed runs")
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name = strings.ReplaceAll(name, "/", "-")
	writePNG(t, filepath.Join(dir, name+".png"), img)
	if err := os.WriteFile(filepath.Join(dir, name+".wav"), wavFile(audio, 48000), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("frame and audio written to %s", dir)
}

// wavFile encodes interleaved 16-bit stereo samples as a WAV file.
func wavFile(samples []int16, rate uint32) []byte {
	var b bytes.Buffer
	size := uint32(len(samples) * 2)
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, 36+size)
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, []any{uint32(16), uint16(1), uint16(2), rate, rate * 4, uint16(4), uint16(16)})
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, size)
	binary.Write(&b, binary.LittleEndian, samples)
	return b.Bytes()
}

// TestBenchROMSceneSelection checks that each button selects its scene,
// through the title the ROM prints on screen (or the HUD of the game).
func TestBenchROMSceneSelection(t *testing.T) {
	for _, s := range benchScenes[:len(benchScenes)-1] {
		g := newBenchROM(t, s, ModelDMG)
		for range 10 { // past the clearing of the WRAM
			g.RunFrame()
		}
		row := 1 // the title, on the second row of the background
		mapBase := 0x1800
		if s.name == "game" {
			row, mapBase = 0, 0x1C00 // the HUD, in the window
		}
		var text strings.Builder
		for col := range 20 {
			text.WriteByte(benchROMChar(g.PPU.vram[mapBase+row*32+col]))
		}
		if !strings.Contains(text.String(), strings.ToUpper(s.name)) {
			t.Errorf("%s: screen reads %q", s.name, text.String())
		}
	}
}

// benchROMChar decodes a tile of the font of the benchmark ROM (see
// benchrom/src/font.asm).
func benchROMChar(tile byte) byte {
	const chars = " 0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ:-./"
	if int(tile) < len(chars) {
		return chars[tile]
	}
	return '?'
}

// goldenRun is goldenHash with the inputs and a callback for the samples.
func goldenRun(g *GameBoy, frames int, inputs func(*GameBoy, int), samples func([]int16)) uint64 {
	h := fnv.New64a()
	for f := range frames {
		inputs(g, f)
		g.RunFrame()
		s := g.APU.DrainSamples()
		if samples != nil {
			samples(s)
		}
		for _, v := range s {
			h.Write([]byte{byte(v), byte(v >> 8)})
		}
		hashFrame(h, g)
	}
	h.Write(g.Snapshot(nil))
	return h.Sum64()
}
