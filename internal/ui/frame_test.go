package ui

import (
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

func benchFrameRGBA(b *testing.B, color, correct bool) {
	console := frameConsole(b, color)
	dst := make([]byte, gb.ScreenWidth*gb.ScreenHeight*4)
	for b.Loop() {
		frameRGBA(dst, console, &Palettes[0], correct)
	}
}

func BenchmarkFrameRGBADMG(b *testing.B)        { benchFrameRGBA(b, false, false) }
func BenchmarkFrameRGBACGB(b *testing.B)        { benchFrameRGBA(b, true, false) }
func BenchmarkFrameRGBACGBCorrect(b *testing.B) { benchFrameRGBA(b, true, true) }
