// Command sitegen builds the project's GitHub Pages site.
//
// It renders site/index.html.tmpl with the releases returned by the GitHub
// API (fetched with the HTML body, see .github/workflows/pages.yml) and
// copies the static files and screenshots next to it.
//
//	gh api -H "Accept: application/vnd.github.html+json" repos/OWNER/REPO/releases > releases.json
//	go run ./tools/sitegen -releases releases.json -out _site
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ldechoux/gbe/internal/ui"
)

// Release is the subset of the GitHub release object used by the site.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	BodyHTML    string    `json:"body_html"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	Assets      []Asset   `json:"assets"`
}

type Asset struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"browser_download_url"`
}

// Download is one archive of the latest release, e.g. macOS arm64.
type Download struct {
	Arch, ArchLabel, URL, Size string
}

// Platform groups the downloads of one OS.
type Platform struct {
	ID, Name, Note string
	Downloads      []Download
}

type page struct {
	Repo      string
	Latest    *Release
	Platforms []Platform
	Releases  []Release
	Palettes  []ui.Palette
	Built     string
}

var platforms = []Platform{
	{ID: "macos", Name: "macOS", Note: "Binaire non signé : au premier lancement, clic droit > Ouvrir."},
	{ID: "windows", Name: "Windows", Note: "Lancer gbe.exe depuis un terminal avec le chemin de la ROM."},
	{ID: "linux", Name: "Linux", Note: "Nécessite libX11, libGL et libasound (présents sur tout bureau)."},
}

var archLabels = map[string]string{
	"amd64": "x86_64",
	"arm64": "arm64",
}

// parseAsset extracts os and arch from "gbe-<tag>-<os>-<arch>.<ext>".
func parseAsset(name string) (osName, arch string, ok bool) {
	base := strings.TrimSuffix(strings.TrimSuffix(name, ".zip"), ".tar.gz")
	parts := strings.Split(base, "-")
	if len(parts) < 4 || parts[0] != "gbe" {
		return "", "", false
	}
	return parts[len(parts)-2], parts[len(parts)-1], true
}

func humanSize(n int64) string {
	return fmt.Sprintf("%.1f Mo", float64(n)/(1024*1024))
}

var months = [...]string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet",
	"août", "septembre", "octobre", "novembre", "décembre"}

func frenchDate(t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()-1], t.Year())
}

// buildPage keeps published releases (newest first, as returned by the API)
// and groups the assets of the latest stable one by platform.
func buildPage(repo string, all []Release) page {
	p := page{Repo: repo, Palettes: ui.Palettes, Built: frenchDate(time.Now())}
	for _, r := range all {
		if r.Draft {
			continue
		}
		p.Releases = append(p.Releases, r)
		if p.Latest == nil && !r.Prerelease {
			latest := r
			p.Latest = &latest
		}
	}
	if p.Latest == nil {
		return p
	}
	for _, plat := range platforms {
		for _, a := range p.Latest.Assets {
			osName, arch, ok := parseAsset(a.Name)
			if !ok || osName != plat.ID {
				continue
			}
			label := archLabels[arch]
			if label == "" {
				label = arch
			}
			plat.Downloads = append(plat.Downloads, Download{arch, label, a.URL, humanSize(a.Size)})
		}
		if len(plat.Downloads) > 0 {
			p.Platforms = append(p.Platforms, plat)
		}
	}
	return p
}

func copyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func main() {
	releasesPath := flag.String("releases", "", "JSON array from the GitHub releases API (empty: no releases)")
	repo := flag.String("repo", "ldechoux/gbe", "owner/name of the GitHub repository")
	siteDir := flag.String("site", "site", "directory with index.html.tmpl and static files")
	shotsDir := flag.String("screenshots", "screenshots", "directory with the screenshots (PNG, WebP)")
	outDir := flag.String("out", "_site", "output directory")
	flag.Parse()

	var releases []Release
	if *releasesPath != "" {
		data, err := os.ReadFile(*releasesPath)
		if err != nil {
			log.Fatal(err)
		}
		if err := json.Unmarshal(data, &releases); err != nil {
			log.Fatalf("parsing %s: %v", *releasesPath, err)
		}
	}

	tmpl, err := template.New("index.html.tmpl").Funcs(template.FuncMap{
		"date": frenchDate,
		// Release bodies are rendered and sanitized by GitHub.
		"trusted": func(s string) template.HTML { return template.HTML(s) },
		"hex": func(c interface{ RGBA() (r, g, b, a uint32) }) string {
			r, g, b, _ := c.RGBA()
			return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
		},
	}).ParseFiles(filepath.Join(*siteDir, "index.html.tmpl"))
	if err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(filepath.Join(*outDir, "index.html"))
	if err != nil {
		log.Fatal(err)
	}
	if err := tmpl.Execute(f, buildPage(*repo, releases)); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}

	// Static files: everything in site/ except the template, plus screenshots.
	copies := map[string]string{}
	entries, err := os.ReadDir(*siteDir)
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() && e.Name() != "index.html.tmpl" {
			copies[filepath.Join(*outDir, e.Name())] = filepath.Join(*siteDir, e.Name())
		}
	}
	var shots []string
	for _, pattern := range []string{"*.png", "*.webp"} {
		m, _ := filepath.Glob(filepath.Join(*shotsDir, pattern))
		shots = append(shots, m...)
	}
	for _, s := range shots {
		copies[filepath.Join(*outDir, "img", filepath.Base(s))] = s
	}
	for dst, src := range copies {
		if err := copyFile(dst, src); err != nil {
			log.Fatal(err)
		}
	}
	// Pages must serve files as-is (no Jekyll processing).
	if err := os.WriteFile(filepath.Join(*outDir, ".nojekyll"), nil, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("site written to %s (%d releases)", *outDir, len(releases))
}
