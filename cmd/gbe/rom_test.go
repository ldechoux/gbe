package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeZip stores the files, in this order, in a zip archive in dir.
func writeZip(t *testing.T, dir string, files ...[2]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(f[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "game.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadROMZip(t *testing.T) {
	path := writeZip(t, t.TempDir(),
		[2]string{"readme.txt", "not a ROM"},
		[2]string{"roms/", ""},
		[2]string{"roms/Game (World).GBC", "rom bytes"},
		[2]string{"other.gb", "second ROM"})
	rom, entry, err := readROM(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(rom) != "rom bytes" || entry != "roms/Game (World).GBC" {
		t.Errorf("got %q from %q, want the first ROM", rom, entry)
	}
}

func TestReadROMZipWithoutROM(t *testing.T) {
	path := writeZip(t, t.TempDir(), [2]string{"readme.txt", "no ROM here"})
	if _, _, err := readROM(path); err == nil || !strings.Contains(err.Error(), "no .gb or .gbc ROM") {
		t.Errorf("err = %v, want no ROM found", err)
	}
}

func TestReadROMZipTooLarge(t *testing.T) {
	big := strings.Repeat("\x00", maxROMSize+1) // compresses to a few KiB
	path := writeZip(t, t.TempDir(), [2]string{"bomb.gb", big})
	if _, _, err := readROM(path); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Errorf("err = %v, want too large", err)
	}
}

func TestReadROMPlain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.gb")
	if err := os.WriteFile(path, []byte("plain ROM"), 0o644); err != nil {
		t.Fatal(err)
	}
	rom, entry, err := readROM(path)
	if err != nil || string(rom) != "plain ROM" || entry != "" {
		t.Errorf("got %q %q %v, want the file as is", rom, entry, err)
	}
}

func TestRomBase(t *testing.T) {
	for path, want := range map[string]string{
		"a/Game.gb":  "a/Game",
		"a/Game.GBC": "a/Game",
		"a/Game.zip": "a/Game",
		"a/Game.ZIP": "a/Game",
		"a/Game.bin": "a/Game.bin",
	} {
		if got := romBase(path); got != want {
			t.Errorf("romBase(%q) = %q, want %q", path, got, want)
		}
	}
}
