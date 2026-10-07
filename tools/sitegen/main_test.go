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
			asset("gbe-v0.1.0-macos-arm64.tar.gz"),
			asset("gbe-v0.1.0-macos-amd64.tar.gz"),
			asset("gbe-v0.1.0-macos-universal.dmg"),
			asset("gbe-v0.1.0-windows-arm64.zip"),
			asset("checksums.txt"),
		}},
	}
	p := buildPage("o/r", releases)
	if len(p.Releases) != 2 {
		t.Fatalf("%d releases listed, drafts must be hidden", len(p.Releases))
	}
	if p.Latest == nil || p.Latest.TagName != "v0.1.0" {
		t.Fatalf("latest = %+v, want the newest stable release", p.Latest)
	}
	var got []string
	for _, pl := range p.Platforms {
		for _, d := range pl.Downloads {
			got = append(got, pl.ID+"/"+d.Arch)
		}
	}
	want := []string{"macos/universal", "macos/arm64", "macos/amd64", "windows/arm64", "linux/amd64"}
	if len(got) != len(want) {
		t.Fatalf("downloads %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("downloads %v, want %v", got, want)
		}
	}
}

// Only the latest releases are shown with their notes; the others are
// linked.
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
	if got := tags(p.Releases); !slices.Equal(got, []string{"v0.5.0", "v0.4.0", "v0.3.1"}) {
		t.Errorf("detailed releases %v", got)
	}
	if got := tags(p.Older); !slices.Equal(got, []string{"v0.3.0", "v0.2.0"}) {
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
