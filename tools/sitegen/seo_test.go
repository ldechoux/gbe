package main

import (
	"encoding/json"
	"encoding/xml"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPagesURL(t *testing.T) {
	if got := pagesURL("LDechoux/gbe"); got != "https://ldechoux.github.io/gbe/" {
		t.Fatal(got)
	}
}

// The sitemap lists the page, with the day it was built.
func TestSitemap(t *testing.T) {
	p := buildPage("o/r", nil)
	var s struct {
		URLs []struct {
			Loc     string `xml:"loc"`
			LastMod string `xml:"lastmod"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal(sitemap(p), &s); err != nil {
		t.Fatal(err)
	}
	if len(s.URLs) != 1 || s.URLs[0].Loc != "https://o.github.io/r/" || s.URLs[0].LastMod != time.Now().Format("2006-01-02") {
		t.Errorf("sitemap %+v", s)
	}
}

// The structured data describe gbe as a free application, at its latest
// version.
func TestStructuredData(t *testing.T) {
	published := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	p := buildPage("o/r", []Release{{TagName: "v0.2.0", PublishedAt: published, HTMLURL: "https://github.com/o/r/releases/tag/v0.2.0"}})
	js, err := structuredData(p)
	if err != nil {
		t.Fatal(err)
	}
	var app map[string]any
	if err := json.Unmarshal([]byte(js), &app); err != nil {
		t.Fatalf("%v:\n%s", err, js)
	}
	for key, want := range map[string]any{
		"@type":           "SoftwareApplication",
		"name":            "gbe",
		"softwareVersion": "0.2.0",
		"datePublished":   "2026-10-01",
		"url":             "https://o.github.io/r/",
		"image":           "https://o.github.io/r/" + shareFile,
	} {
		if app[key] != want {
			t.Errorf("%s = %v, want %v", key, app[key], want)
		}
	}
	if offer, _ := app["offers"].(map[string]any); offer["price"] != "0" {
		t.Errorf("offers %v: gbe is free", app["offers"])
	}
}

// The page tells the search engines and the social networks where it is,
// what it is about and which image to show; the verification tag of the
// Search Console only when its token is given.
func TestRenderedHead(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the site")
	}
	root := filepath.Join("..", "..")
	for _, token := range []string{"", "abc123"} {
		out := t.TempDir()
		cmd := exec.Command("go", "run", ".", "-site", filepath.Join(root, "site"), "-screenshots", filepath.Join(root, "screenshots"),
			"-icon", filepath.Join(root, "assets", "icon", "icon-512.png"), "-out", out, "-google-verification", token)
		if msg, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("sitegen: %v\n%s", err, msg)
		}
		html := readFile(t, filepath.Join(out, "index.html"))
		for _, want := range []string{
			`<link rel="canonical" href="https://ldechoux.github.io/gbe/">`,
			`<meta property="og:image" content="https://ldechoux.github.io/gbe/img/share.png">`,
			`<meta name="twitter:card" content="summary_large_image">`,
			`<link rel="apple-touch-icon" href="apple-touch-icon.png">`,
			`"@type": "SoftwareApplication"`,
		} {
			if !strings.Contains(html, want) {
				t.Errorf("token %q: %s missing", token, want)
			}
		}
		if got := strings.Contains(html, `<meta name="google-site-verification" content="abc123">`); got != (token != "") {
			t.Errorf("token %q: verification tag present %v", token, got)
		}
		for _, name := range []string{"sitemap.xml", shareFile, touchFile} {
			if readFile(t, filepath.Join(out, name)) == "" {
				t.Errorf("%s is empty", name)
			}
		}
	}
}

// The image of a shared link has the size the networks expect, the
// screens of both consoles in it.
func TestShareImage(t *testing.T) {
	img, err := shareImage(filepath.Join("..", "..", "screenshots"))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != shareWidth || b.Dy() != shareHeight {
		t.Fatalf("size %v", b)
	}
	// The background, then the bezels of the two consoles, at the right.
	for _, c := range []struct {
		x, y int
		want color.RGBA
	}{
		{10, 10, shareBg},
		{570, 60, shareBezel},
		{1130, 550, shareBezelGBC},
	} {
		if got := img.RGBAAt(c.x, c.y); got != c.want {
			t.Errorf("(%d, %d): %v, want %v", c.x, c.y, got, c.want)
		}
	}
	icon, err := touchIcon(filepath.Join("..", "..", "assets", "icon", "icon-512.png"))
	if err != nil {
		t.Fatal(err)
	}
	if b := icon.Bounds(); b.Dx() != touchSize || icon.RGBAAt(0, 0) != (color.RGBA{0xe6, 0xe4, 0xdc, 0xff}) {
		t.Errorf("touch icon %v, corner %v", b, icon.RGBAAt(0, 0))
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
