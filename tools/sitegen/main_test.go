package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBuildPage(t *testing.T) {
	asset := func(name string) Asset { return Asset{Name: name, Size: 3 << 20, URL: "https://x/" + name} }
	releases := []Release{
		{TagName: "v0.3.0", Draft: true},
		{TagName: "v0.2.0-rc1", Prerelease: true, PublishedAt: time.Now()},
		{TagName: "v0.1.0", PublishedAt: time.Now(), Assets: []Asset{
			asset("gbe-v0.1.0-linux-amd64.tar.gz"),
			asset("gbe-v0.1.0-linux-amd64.AppImage"),
			asset("gbe-v0.1.0-macos-arm64.tar.gz"),
			asset("gbe-v0.1.0-macos-amd64.tar.gz"),
			asset("gbe-v0.1.0-macos-universal.dmg"),
			asset("gbe-v0.1.0-windows-arm64.zip"),
			asset("checksums.txt"),
		}},
	}
	p := buildPage("o/r", releases)
	if n := len(p.Releases) + len(p.Older); n != 2 {
		t.Fatalf("%d releases listed, drafts must be hidden", n)
	}
	if p.Latest == nil || p.Latest.TagName != "v0.1.0" {
		t.Fatalf("latest = %+v, want the newest stable release", p.Latest)
	}
	var got []string
	for _, pl := range p.Platforms {
		for _, d := range pl.Downloads {
			got = append(got, pl.ID+"/"+d.ArchLabel)
		}
	}
	want := []string{"macos/Application", "macos/arm64", "macos/x86_64", "windows/arm64", "linux/AppImage x86_64", "linux/x86_64"}
	if len(got) != len(want) {
		t.Fatalf("downloads %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("downloads %v, want %v", got, want)
		}
	}
}

// Only the latest release is shown with its notes; the others are linked.
func TestBuildPageOlderReleases(t *testing.T) {
	var releases []Release
	for _, tag := range []string{"v0.5.0", "v0.4.0", "v0.3.1", "v0.3.0", "v0.2.0"} {
		releases = append(releases, Release{TagName: tag, PublishedAt: time.Now()})
	}
	releases = append([]Release{{TagName: "v0.6.0", Draft: true}}, releases...)
	p := buildPage("o/r", releases)
	tags := func(rs []Release) (out []string) {
		for _, r := range rs {
			out = append(out, r.TagName)
		}
		return out
	}
	if got := tags(p.Releases); !slices.Equal(got, []string{"v0.5.0"}) {
		t.Errorf("detailed releases %v", got)
	}
	if got := tags(p.Older); !slices.Equal(got, []string{"v0.4.0", "v0.3.1", "v0.3.0", "v0.2.0"}) {
		t.Errorf("older releases %v", got)
	}
	if p.Latest == nil || p.Latest.TagName != "v0.5.0" {
		t.Errorf("latest %+v", p.Latest)
	}
}

// The links of the release notes open in a new tab, like the other links
// that leave the page.
func TestNewTabLinks(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`<a href="https://x.io" rel="nofollow">x</a>`, `<a target="_blank" href="https://x.io" rel="nofollow noopener">x</a>`},
		{`<a class="commit-link" href="https://github.com/o/r/commit/1">1</a>`, `<a target="_blank" rel="noopener" class="commit-link" href="https://github.com/o/r/commit/1">1</a>`},
		{`<a href="#notes">here</a>`, `<a href="#notes">here</a>`},
		{`<a href="https://x.io" target="_self">x</a>`, `<a href="https://x.io" target="_self">x</a>`},
		{`<a href="https://x.io" rel="noopener">x</a>`, `<a target="_blank" href="https://x.io" rel="noopener">x</a>`},
		{`<p>no link</p>`, `<p>no link</p>`},
	} {
		if got := newTabLinks(c.in); got != c.want {
			t.Errorf("newTabLinks(%s)\n got %s\nwant %s", c.in, got, c.want)
		}
	}
	p := buildPage("o/r", []Release{{TagName: "v1", BodyHTML: `<a href="https://x.io">x</a>`}})
	if !strings.Contains(p.Releases[0].BodyHTML, `target="_blank"`) {
		t.Errorf("release notes links: %s", p.Releases[0].BodyHTML)
	}
}

func TestFrenchDate(t *testing.T) {
	if got := frenchDate(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)); got != "1 août 2026" {
		t.Fatal(got)
	}
}

func TestScreenshots(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.png", "gb/b.webp", "gbc/c.WEBP", "gbc/notes.txt"} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := screenshots(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.png", filepath.Join("gb", "b.webp"), filepath.Join("gbc", "c.WEBP")}
	if !slices.Equal(got, want) {
		t.Fatalf("screenshots %v, want %v", got, want)
	}
}

// siteArgs are the arguments that build the site of the repository into
// out.
func siteArgs(out string, more ...string) []string {
	root := filepath.Join("..", "..")
	return append([]string{"-site", filepath.Join(root, "site"), "-screenshots", filepath.Join(root, "screenshots"),
		"-icon", filepath.Join(root, "assets", "icon", "icon-512.png"), "-out", out}, more...)
}

// With the releases of the GitHub API, the page offers the downloads of
// the latest one and shows its notes.
func TestRunWithReleases(t *testing.T) {
	dir := t.TempDir()
	releases := filepath.Join(dir, "releases.json")
	data := `[{"tag_name": "v0.9.0", "name": "gbe v0.9.0", "html_url": "https://github.com/o/r/releases/tag/v0.9.0",
		"published_at": "2026-10-01T10:00:00Z", "body_html": "<p>Notes with <a href=\"https://x.io\">a link</a></p>",
		"assets": [{"name": "gbe-v0.9.0-macos-universal.dmg", "size": 5242880, "browser_download_url": "https://x/gbe.dmg"}]}]`
	if err := os.WriteFile(releases, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "site")
	if err := run(siteArgs(out, "-releases", releases, "-url", "https://gbe.example.org")); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`href="https://x/gbe.dmg"`, "5.0 Mo", "Télécharger v0.9.0",
		`<a target="_blank" rel="noopener" href="https://x.io">a link</a>`,
		`<link rel="canonical" href="https://gbe.example.org/">`, `"softwareVersion": "0.9.0"`,
	} {
		if !strings.Contains(string(html), want) {
			t.Errorf("%s missing", want)
		}
	}
	for _, name := range []string{"style.css", "favicon.svg", "img/gb/screenshot.webp", ".nojekyll"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// What cannot be read stops the build, with the reason.
func TestRunErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"-nope"}, "flag provided but not defined"},
		{siteArgs(out, "-releases", filepath.Join(dir, "missing.json")), "missing.json"},
		{siteArgs(out, "-releases", bad), "parsing"},
		{siteArgs(out, "-site", filepath.Join(dir, "nosite")), "index.html.tmpl"},
		{siteArgs(out, "-screenshots", filepath.Join(dir, "noshots")), "noshots"},
		{siteArgs(out, "-icon", filepath.Join(dir, "noicon.png")), "noicon.png"},
	} {
		err := run(c.args)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.args[len(c.args)-1], err, c.want)
		}
	}
}

func TestCopyFileErrors(t *testing.T) {
	dir := t.TempDir()
	if err := copyFile(filepath.Join(dir, "out"), filepath.Join(dir, "missing")); err == nil {
		t.Error("copied a missing file")
	}
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(filepath.Join(src, "under-a-file"), src); err == nil {
		t.Error("copied under a file")
	}
}
