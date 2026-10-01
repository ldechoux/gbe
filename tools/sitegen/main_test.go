package main

import (
	"os"
	"path/filepath"
	"slices"
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
	want := []string{"macos/arm64", "macos/amd64", "windows/arm64", "linux/amd64"}
	if len(got) != len(want) {
		t.Fatalf("downloads %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("downloads %v, want %v", got, want)
		}
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
