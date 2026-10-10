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
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

// Download is one file of the latest release, e.g. the macOS arm64
// archive. App is set for an application (the macOS disk image, the Linux
// AppImage), as opposed to an archive of the binary alone.
type Download struct {
	Arch, ArchLabel, URL, Size string
	App                        bool
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
	Releases  []Release // the latest ones, shown with their notes
	Older     []Release // the others, linked to their GitHub page
	Palettes  []ui.Palette
	Built     string

	// For the search engines and the shared links (see seo.go).
	URL          string // where the site is served, ending with /
	Owner        string // of the repository
	Updated      string // the day the page was built, as 2006-01-02
	Description  string
	ShareImage   string // path of the image of a shared link
	Verification string // the Google Search Console token, "" for none
	Structured   template.JS
}

// detailedReleases is how many releases, the newest, the page shows with
// their notes: the others are only linked, to keep the page short.
const detailedReleases = 3

var platforms = []Platform{
	{ID: "macos", Name: "macOS", Note: "Glisser gbe dans Applications. L'app n'est pas signée par Apple : au premier lancement, Réglages Système > Confidentialité et sécurité > Ouvrir quand même. Les archives x86_64 et arm64 contiennent le binaire seul, pour le terminal."},
	{ID: "windows", Name: "Windows", Note: "Décompresser l'archive et double-cliquer sur gbe.exe. L'exécutable n'est pas signé : si Windows SmartScreen l'arrête, cliquer sur Informations complémentaires > Exécuter quand même."},
	{ID: "linux", Name: "Linux", Note: "L'AppImage se lance d'un double-clic, après l'avoir rendue exécutable (Propriétés > Permissions, ou chmod +x). Nécessite libX11, libGL et libasound, présents sur tout bureau."},
}

var archLabels = map[string]string{
	"amd64":     "x86_64",
	"arm64":     "arm64",
	"universal": "Application",
}

// parseAsset extracts os and arch from "gbe-<tag>-<os>-<arch>.<ext>". The
// application for macOS, for both architectures, is "universal".
func parseAsset(name string) (osName, arch string, ok bool) {
	base := name
	for _, ext := range []string{".zip", ".tar.gz", ".dmg", ".AppImage"} {
		base = strings.TrimSuffix(base, ext)
	}
	parts := strings.Split(base, "-")
	if len(parts) < 4 || parts[0] != "gbe" {
		return "", "", false
	}
	return parts[len(parts)-2], parts[len(parts)-1], true
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
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
	now := time.Now()
	owner, _, _ := strings.Cut(repo, "/")
	p := page{Repo: repo, Palettes: ui.Palettes, Built: frenchDate(now),
		URL: pagesURL(repo), Owner: owner, Updated: now.Format("2006-01-02"),
		Description: description, ShareImage: shareFile}
	for _, r := range all {
		if r.Draft {
			continue
		}
		r.BodyHTML = newTabLinks(r.BodyHTML)
		p.Releases = append(p.Releases, r)
		if p.Latest == nil && !r.Prerelease {
			latest := r
			p.Latest = &latest
		}
	}
	if len(p.Releases) > detailedReleases {
		p.Releases, p.Older = p.Releases[:detailedReleases], p.Releases[detailedReleases:]
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
			appImage := strings.HasSuffix(a.Name, ".AppImage")
			if appImage {
				label = "AppImage " + label
			}
			app := appImage || strings.HasSuffix(a.Name, ".dmg")
			plat.Downloads = append(plat.Downloads, Download{arch, label, a.URL, humanSize(a.Size), app})
		}
		// The applications come before the archives of the binary alone.
		slices.SortStableFunc(plat.Downloads, func(a, b Download) int {
			return cmp.Compare(boolInt(b.App), boolInt(a.App))
		})
		if len(plat.Downloads) > 0 {
			p.Platforms = append(p.Platforms, plat)
		}
	}
	return p
}

var (
	anchorTag = regexp.MustCompile(`<a\b[^>]*>`)
	relAttr   = regexp.MustCompile(`\brel="([^"]*)"`)
)

// newTabLinks makes the links of release notes open in a new tab, like the
// other links of the page that leave it: target="_blank", and rel="noopener"
// added to the rel GitHub may already give them (e.g. "nofollow").
func newTabLinks(html string) string {
	return anchorTag.ReplaceAllStringFunc(html, func(tag string) string {
		if !strings.Contains(tag, `href="http`) || strings.Contains(tag, "target=") {
			return tag
		}
		if m := relAttr.FindStringSubmatch(tag); m != nil {
			if !slices.Contains(strings.Fields(m[1]), "noopener") {
				tag = strings.Replace(tag, m[0], `rel="`+strings.TrimSpace(m[1]+" noopener")+`"`, 1)
			}
		} else {
			tag = `<a rel="noopener"` + tag[len("<a"):]
		}
		return `<a target="_blank"` + tag[len("<a"):]
	})
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

// screenshots lists the PNG and WebP images under dir, subdirectories
// included (one per hardware mode, e.g. gb/ and gbc/), relative to dir.
func screenshots(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		if d.IsDir() || ext != ".png" && ext != ".webp" {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		out = append(out, rel)
		return err
	})
	return out, err
}

func main() {
	releasesPath := flag.String("releases", "", "JSON array from the GitHub releases API (empty: no releases)")
	repo := flag.String("repo", "ldechoux/gbe", "owner/name of the GitHub repository")
	siteDir := flag.String("site", "site", "directory with index.html.tmpl and static files")
	shotsDir := flag.String("screenshots", "screenshots", "directory with the screenshots (PNG, WebP, subdirectories included)")
	outDir := flag.String("out", "_site", "output directory")
	siteURL := flag.String("url", "", "address of the site, ending with / (default: the GitHub Pages one of -repo)")
	verification := flag.String("google-verification", "", "Google Search Console verification token (the content of its meta tag)")
	iconPath := flag.String("icon", "assets/icon/icon-512.png", "application icon, for the icon of a home screen")
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
	p := buildPage(*repo, releases)
	if *siteURL != "" {
		p.URL = strings.TrimSuffix(*siteURL, "/") + "/"
	}
	p.Verification = *verification
	if p.Structured, err = structuredData(p); err != nil {
		log.Fatal(err)
	}
	if err := tmpl.Execute(f, p); err != nil {
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
	shots, err := screenshots(*shotsDir)
	if err != nil {
		log.Fatal(err)
	}
	for _, rel := range shots {
		copies[filepath.Join(*outDir, "img", rel)] = filepath.Join(*shotsDir, rel)
	}
	for dst, src := range copies {
		if err := copyFile(dst, src); err != nil {
			log.Fatal(err)
		}
	}
	share, err := shareImage(*shotsDir)
	if err == nil {
		err = writePNG(filepath.Join(*outDir, shareFile), share)
	}
	if err != nil {
		log.Fatal(err)
	}
	icon, err := touchIcon(*iconPath)
	if err == nil {
		err = writePNG(filepath.Join(*outDir, touchFile), icon)
	}
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*outDir, "sitemap.xml"), sitemap(p), 0o644); err != nil {
		log.Fatal(err)
	}
	// Pages must serve files as-is (no Jekyll processing).
	if err := os.WriteFile(filepath.Join(*outDir, ".nojekyll"), nil, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("site written to %s (%d releases)", *outDir, len(releases))
}
