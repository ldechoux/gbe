package ui

import (
	"bytes"
	"testing"

	"github.com/ldechoux/gbe/internal/gb"
)

// frameConsole returns a console of the given kind whose last frame holds
// every shade, or a spread of colors.
func frameConsole(tb testing.TB, color bool) *gb.GameBoy {
	tb.Helper()
	rom := make([]byte, 0x8000)
	if color {
		rom[0x143] = 0xC0
	}
	cart, err := gb.NewCartridge(rom)
	if err != nil {
		tb.Fatal(err)
	}
	console, err := gb.New(cart, nil)
	if err != nil {
		tb.Fatal(err)
	}
	for i := range console.Framebuffer() {
		console.Framebuffer()[i] = byte(i % 4)
		console.ColorFramebuffer()[i] = uint16(i*2731) & 0x7FFF
	}
	return console
}

// TestFrameRGBA checks the conversion of every pixel against rgb555 and
// the palette.
func TestFrameRGBA(t *testing.T) {
	dst := make([]byte, screenPixels*4)
	for _, correct := range []bool{false, true} {
		console := frameConsole(t, true)
		fb := console.ColorFramebuffer()
		for c := range 1 << 15 { // every color, in several frames
			fb[c%screenPixels] = uint16(c)
			if c%screenPixels != screenPixels-1 && c != 1<<15-1 {
				continue
			}
			frameRGBA(dst, consoleFrame(console), &Palettes[0], correct)
			for i, v := range fb {
				r, g, b := rgb555(v, correct)
				if got, want := dst[i*4:i*4+4], []byte{r, g, b, 0xFF}; !bytes.Equal(got, want) {
					t.Fatalf("color %04X (correct=%v): %v, want %v", v, correct, got, want)
				}
			}
		}
	}
	console := frameConsole(t, false)
	for _, pal := range Palettes {
		frameRGBA(dst, consoleFrame(console), &pal, false)
		for i, s := range console.Framebuffer() {
			c := pal.Colors[s]
			if got, want := dst[i*4:i*4+4], []byte{c.R, c.G, c.B, 0xFF}; !bytes.Equal(got, want) {
				t.Fatalf("%s shade %d: %v, want %v", pal.ID, s, got, want)
			}
		}
	}
}

func TestLCDFrameUpdate(t *testing.T) {
	var f lcdFrame
	dmg, cgb := frameConsole(t, false), frameConsole(t, true)
	steps := []struct {
		what    string
		console *gb.GameBoy
		pal     *Palette
		correct bool
		change  func()
		want    bool
	}{
		{"first frame", dmg, &Palettes[0], false, nil, true},
		{"same frame", dmg, &Palettes[0], false, nil, false},
		{"correction (DMG)", dmg, &Palettes[0], true, nil, false},
		{"palette", dmg, &Palettes[1], true, nil, true},
		{"new frame", dmg, &Palettes[1], true, func() { dmg.Framebuffer()[100]++ }, true},
		{"color console", cgb, &Palettes[1], true, nil, true},
		{"same color frame", cgb, &Palettes[1], true, nil, false},
		{"palette (CGB)", cgb, &Palettes[0], true, nil, false},
		{"correction", cgb, &Palettes[0], false, nil, true},
		{"new color frame", cgb, &Palettes[0], false, func() { cgb.ColorFramebuffer()[5]++ }, true},
		{"back to DMG", dmg, &Palettes[0], false, nil, true},
	}
	for _, s := range steps {
		if s.change != nil {
			s.change()
		}
		if got := f.update(consoleFrame(s.console), s.pal, s.correct); got != s.want {
			t.Errorf("%s: update reported %v", s.what, got)
		}
		want := make([]byte, screenPixels*4)
		frameRGBA(want, consoleFrame(s.console), s.pal, s.correct)
		if !bytes.Equal(f.pix, want) {
			t.Errorf("%s: wrong pixels", s.what)
		}
	}
}

func benchFrameRGBA(b *testing.B, color, correct bool) {
	console := frameConsole(b, color)
	dst := make([]byte, gb.ScreenWidth*gb.ScreenHeight*4)
	for b.Loop() {
		frameRGBA(dst, consoleFrame(console), &Palettes[0], correct)
	}
}

func BenchmarkFrameRGBADMG(b *testing.B)        { benchFrameRGBA(b, false, false) }
func BenchmarkFrameRGBACGB(b *testing.B)        { benchFrameRGBA(b, true, false) }
func BenchmarkFrameRGBACGBCorrect(b *testing.B) { benchFrameRGBA(b, true, true) }
