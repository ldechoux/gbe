package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ldechoux/gbe/internal/gb"
)

// DefaultScreenshotDir is ~/Pictures/gbe.
func DefaultScreenshotDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "screenshots"
	}
	return filepath.Join(home, "Pictures", "gbe")
}

// Screenshot renders the last frame of the console (DMG shades through pal,
// CGB colors corrected or not), each Game Boy pixel becoming a scale x scale
// block.
func Screenshot(console *gb.GameBoy, pal *Palette, correct bool, scale int) *image.RGBA {
	pix := make([]byte, gb.ScreenWidth*gb.ScreenHeight*4)
	frameRGBA(pix, console, pal, correct)
	return scaleImage(pix, scale)
}

// scaleImage turns a 160x144 RGBA frame into an image scaled scale times.
func scaleImage(pix []byte, scale int) *image.RGBA {
	scale = max(1, scale)
	img := image.NewRGBA(image.Rect(0, 0, gb.ScreenWidth*scale, gb.ScreenHeight*scale))
	for y := range gb.ScreenHeight * scale {
		row := img.Pix[y*img.Stride:]
		for x := range gb.ScreenWidth * scale {
			copy(row[x*4:x*4+4], pix[((y/scale)*gb.ScreenWidth+x/scale)*4:])
		}
	}
	return img
}

// screenshotName builds "<title>-YYYYMMDD-HHMMSS.png" from the game title.
func screenshotName(title string, t time.Time) string {
	slug := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 'a' - 'A'
		}
		return '-'
	}, strings.TrimSpace(title))
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "gbe"
	}
	return fmt.Sprintf("%s-%s.png", slug, t.Format("20060102-150405"))
}

// saveScreenshot writes img into dir without overwriting an existing file
// and returns the path used.
func saveScreenshot(dir, title string, img image.Image, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := strings.TrimSuffix(screenshotName(title, now), ".png")
	for i := 1; ; i++ {
		name := base + ".png"
		if i > 1 {
			name = fmt.Sprintf("%s-%d.png", base, i)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			return "", err
		}
		return path, f.Close()
	}
}
