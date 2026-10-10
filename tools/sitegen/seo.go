package main

// What the site gives the search engines and the social networks: the
// image shown when a link to it is shared, the icon of a home screen, the
// sitemap, and the description of the software as structured data.

import (
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/webp"
)

// description is the summary of the page, for the search engines and the
// shared links.
const description = "gbe, un émulateur Game Boy et Game Boy Color écrit en Go pur pour macOS, Linux et Windows : couleurs, palettes, filtres pour grand écran jusqu'en 4K, vibrations des manettes, touches configurables, save states, captures d'écran."

// The image shown with a shared link (Open Graph), at the size the social
// networks expect.
const (
	shareWidth  = 1200
	shareHeight = 630
	shareFile   = "img/share.png"
	touchFile   = "apple-touch-icon.png"
	touchSize   = 180 // what iOS asks for
)

// Colors of the site (site/style.css): the light theme, and the bezels of
// the two consoles.
var (
	shareBg       = color.RGBA{0xe6, 0xe4, 0xdc, 0xff}
	shareInk      = color.RGBA{0x1f, 0x2a, 0x1f, 0xff}
	shareMuted    = color.RGBA{0x5a, 0x63, 0x56, 0xff}
	shareShadow   = color.RGBA{0xb9, 0xb6, 0xa8, 0xff}
	shareBezel    = color.RGBA{0x5b, 0x5d, 0x6b, 0xff}
	shareBezelGBC = color.RGBA{0x4b, 0x3a, 0x78, 0xff}
	touchBg       = shareBg
)

// shareImage draws the image of a shared link, like the top of the site:
// the name and what gbe is at the left, in the pixel font of its menus,
// and at the right the title screens of the two consoles (screenshots/gb
// and gbc) in their bezels, one over the other.
func shareImage(shotsDir string) (*image.RGBA, error) {
	img := image.NewRGBA(image.Rect(0, 0, shareWidth, shareHeight))
	draw.Draw(img, img.Bounds(), &image.Uniform{shareBg}, image.Point{}, draw.Src)

	pixelText(img, "gbe", 16, 64, 92, shareInk)
	for i, line := range []string{"Émulateur", "Game Boy et", "Game Boy Color"} {
		pixelText(img, line, 4, 70, 338+i*52, shareInk)
	}
	for i, line := range []string{"pour macOS, Linux", "et Windows"} {
		pixelText(img, line, 3, 70, 512+i*42, shareMuted)
	}

	// The screens at x2, a pixel of the console 2x2 pixels.
	const scale = 2
	for i, shot := range []struct {
		path  string
		bezel color.RGBA
	}{
		{filepath.Join(shotsDir, "gb", "screenshot.webp"), shareBezel},
		{filepath.Join(shotsDir, "gbc", "screenshot.webp"), shareBezelGBC},
	} {
		screen, err := decodeImage(shot.path)
		if err != nil {
			return nil, err
		}
		x, y := 560+i*200, 50+i*170
		fill(img, image.Rect(x+12, y+12, x+392, y+356), shareShadow)
		fill(img, image.Rect(x, y, x+380, y+344), shot.bezel)
		dst := image.Rect(x+30, y+28, x+30+160*scale, y+28+144*scale)
		xdraw.NearestNeighbor.Scale(img, dst, screen, screen.Bounds(), draw.Src, nil)
	}
	return img, nil
}

func fill(img *image.RGBA, r image.Rectangle, c color.Color) {
	draw.Draw(img, r, &image.Uniform{c}, image.Point{}, draw.Src)
}

// pixelText draws s in the 7x13 bitmap font of the menus of gbe, each of
// its pixels scale x scale, from (x, y), the top of the line. The font is
// ASCII only: an É is an E with its accent added.
func pixelText(img *image.RGBA, s string, scale, x, y int, c color.Color) {
	runes := []rune(s)
	small := image.NewAlpha(image.Rect(0, 0, 7*len(runes), 13))
	d := &font.Drawer{Dst: small, Src: image.NewUniform(color.Alpha{0xff}), Face: basicfont.Face7x13, Dot: fixed.P(0, 11)}
	for i, r := range runes {
		if r == 'É' {
			small.SetAlpha(7*i+3, 1, color.Alpha{0xff}) // the accent, above the E
			small.SetAlpha(7*i+4, 0, color.Alpha{0xff})
			r = 'E'
		}
		d.DrawString(string(r))
	}
	dst := image.Rect(x, y, x+small.Bounds().Dx()*scale, y+13*scale)
	big := image.NewAlpha(dst)
	xdraw.NearestNeighbor.Scale(big, dst, small, small.Bounds(), draw.Src, nil)
	draw.DrawMask(img, dst, &image.Uniform{c}, image.Point{}, big, dst.Min, draw.Over)
}

// touchIcon is the icon of a home screen (apple-touch-icon), from the
// application icon, on the background of the site: iOS would fill its
// transparent corners with black.
func touchIcon(iconPath string) (*image.RGBA, error) {
	src, err := decodeImage(iconPath)
	if err != nil {
		return nil, err
	}
	img := image.NewRGBA(image.Rect(0, 0, touchSize, touchSize))
	draw.Draw(img, img.Bounds(), &image.Uniform{touchBg}, image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(img, img.Bounds(), src, src.Bounds(), draw.Over, nil)
	return img, nil
}

func decodeImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var img image.Image
	if strings.EqualFold(filepath.Ext(path), ".webp") {
		img, err = webp.Decode(f)
	} else {
		img, err = png.Decode(f)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return img, nil
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// sitemap lists the page for the search engines, with the day it was last
// built: the site is built again whenever it changes.
func sitemap(p page) []byte {
	return fmt.Appendf(nil, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>%s</loc>
    <lastmod>%s</lastmod>
  </url>
</urlset>
`, p.URL, p.Updated)
}

// structuredData describes gbe as a free application (schema.org
// SoftwareApplication), for the search engines: its systems, its latest
// version and where to download it.
func structuredData(p page) (template.JS, error) {
	app := map[string]any{
		"@context":               "https://schema.org",
		"@type":                  "SoftwareApplication",
		"name":                   "gbe",
		"description":            description,
		"url":                    p.URL,
		"inLanguage":             "fr",
		"applicationCategory":    "GameApplication",
		"applicationSubCategory": "Émulateur",
		"operatingSystem":        "macOS, Windows, Linux",
		"image":                  p.URL + shareFile,
		"screenshot":             []string{p.URL + "img/gb/screenshot.webp", p.URL + "img/gbc/screenshot.webp"},
		"downloadUrl":            p.URL + "#telecharger",
		"offers":                 map[string]any{"@type": "Offer", "price": "0", "priceCurrency": "EUR"},
		"sameAs":                 []string{"https://github.com/" + p.Repo},
		"author":                 map[string]any{"@type": "Person", "name": p.Owner, "url": "https://github.com/" + p.Owner},
	}
	if p.Latest != nil {
		app["softwareVersion"] = strings.TrimPrefix(p.Latest.TagName, "v")
		app["datePublished"] = p.Latest.PublishedAt.Format("2006-01-02")
		app["releaseNotes"] = p.Latest.HTMLURL
	}
	data, err := json.MarshalIndent(app, "", "  ")
	return template.JS(data), err
}

// pagesURL is the address GitHub Pages serves the site of repo at.
func pagesURL(repo string) string {
	owner, name, _ := strings.Cut(repo, "/")
	return fmt.Sprintf("https://%s.github.io/%s/", strings.ToLower(owner), name)
}
