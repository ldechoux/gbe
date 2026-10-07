package rom

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ldechoux/gbe/internal/gb"
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
	rom, entry, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(rom) != "rom bytes" || entry != "roms/Game (World).GBC" {
		t.Errorf("got %q from %q, want the first ROM", rom, entry)
	}
}

func TestReadROMZipWithoutROM(t *testing.T) {
	path := writeZip(t, t.TempDir(), [2]string{"readme.txt", "no ROM here"})
	if _, _, err := Read(path); !errors.Is(err, ErrNoROMInZip) {
		t.Errorf("err = %v, want no ROM found", err)
	}
}

func TestReadROMZipTooLarge(t *testing.T) {
	big := strings.Repeat("\x00", maxROMSize+1) // compresses to a few KiB
	path := writeZip(t, t.TempDir(), [2]string{"bomb.gb", big})
	if _, _, err := Read(path); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want too large", err)
	}
}

func TestReadROMPlain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.gb")
	if err := os.WriteFile(path, []byte("plain ROM"), 0o644); err != nil {
		t.Fatal(err)
	}
	rom, entry, err := Read(path)
	if err != nil || string(rom) != "plain ROM" || entry != "" {
		t.Errorf("got %q %q %v, want the file as is", rom, entry, err)
	}
}

func TestBase(t *testing.T) {
	for path, want := range map[string]string{
		"a/Game.gb":  "a/Game",
		"a/Game.GBC": "a/Game",
		"a/Game.zip": "a/Game",
		"a/Game.ZIP": "a/Game",
		"a/Game.bin": "a/Game.bin",
	} {
		if got := Base(path); got != want {
			t.Errorf("Base(%q) = %q, want %q", path, got, want)
		}
	}
}

// cartridge is a 32 KiB ROM titled title, of cartridge type typ (0x03: MBC1
// with a battery and 8 KiB of RAM).
func cartridge(title string, typ byte) []byte {
	rom := make([]byte, 0x8000)
	copy(rom[0x134:], title)
	rom[0x147], rom[0x149] = typ, 0x02
	rom[0x100] = 0x18 // JR -2: loops forever
	rom[0x101] = 0xFE
	return rom
}

func writeFile(t *testing.T, path string, data []byte) string {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpen(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, filepath.Join(dir, "Game.gb"), cartridge("TEST GAME", 0x03))
	save := bytes.Repeat([]byte{0x42}, 0x2000)
	writeFile(t, filepath.Join(dir, "Game.sav"), save)

	g, err := Open(path, Options{Model: gb.ModelAuto, BootROM: "none", Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if g.Title != "TEST GAME" || g.SavePath != filepath.Join(dir, "Game.sav") ||
		g.StatePath != filepath.Join(dir, "Game.state") || !g.AutoModel || g.Path != path {
		t.Errorf("game %+v", g)
	}
	if g.Console.IsCGB() {
		t.Error("a DMG game runs on a DMG, unless colorized")
	}
	if got := g.Console.Cart.SaveData(); !bytes.Equal(got[:len(save)], save) {
		t.Error("the battery save is not loaded")
	}

	g, err = Open(path, Options{Model: gb.ModelAuto, BootROM: "none", Colorize: true})
	if err != nil || !g.Console.IsCGB() {
		t.Errorf("colorized: %v, CGB %v", err, err == nil && g.Console.IsCGB())
	}
	if c, err := g.NewConsole(gb.ModelDMG); err != nil || c.IsCGB() {
		t.Errorf("NewConsole(DMG): %v", err)
	}
}

func TestOpenZip(t *testing.T) {
	dir := t.TempDir()
	path := writeZip(t, dir, [2]string{"Game.gbc", string(cartridge("ZIPPED", 0x00))})
	g, err := Open(path, Options{BootROM: "none", Strict: true})
	if err != nil || g.Title != "ZIPPED" || g.StatePath != filepath.Join(dir, "game.state") {
		t.Errorf("zip: %v %+v", err, g)
	}
}

func TestOpenErrors(t *testing.T) {
	dir := t.TempDir()
	strict := Options{BootROM: "none", Strict: true}
	for _, tc := range []struct {
		name string
		path string
		opts Options
		want error
	}{
		{"unknown extension", writeFile(t, filepath.Join(dir, "notes.txt"), cartridge("TXT", 0)), strict, ErrNotROM},
		{"zip without ROM", writeZip(t, dir, [2]string{"readme.txt", "no ROM"}), strict, ErrNoROMInZip},
		{"too small", writeFile(t, filepath.Join(dir, "tiny.gb"), []byte("tiny")), strict, gb.ErrROMTooSmall},
		{"too large", writeFile(t, filepath.Join(dir, "big.gb"), make([]byte, maxROMSize+1)), strict, ErrTooLarge},
		{"missing", filepath.Join(dir, "missing.gb"), strict, fs.ErrNotExist},
	} {
		if _, err := Open(tc.path, tc.opts); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}

	path := writeFile(t, filepath.Join(dir, "huc.gb"), cartridge("HUC", 0x22))
	var typ gb.CartridgeTypeError
	if _, err := Open(path, strict); !errors.As(err, &typ) || typ != 0x22 {
		t.Errorf("unsupported cartridge: err = %v", err)
	}

	// The command line opens any file, whatever its extension.
	path = writeFile(t, filepath.Join(dir, "game.bin"), cartridge("BIN", 0))
	if g, err := Open(path, Options{BootROM: "none"}); err != nil || g.StatePath != path+".state" {
		t.Errorf("not strict: %v", err)
	}
}

// The default boot ROM comes from the first folder that has it; a missing
// one is not an error.
func TestReadBootROM(t *testing.T) {
	empty, first, second := t.TempDir(), t.TempDir(), t.TempDir()
	write := func(dir, name, data string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(first, "gb_bios.bin", "first gb")
	write(second, "gb_bios.bin", "second gb")
	write(second, "gbc_bios.bin", "second gbc")
	opts := Options{BootROMs: &BootROMSearch{Defaults: []string{empty, first, second}}}
	for _, c := range []struct {
		opts  Options
		model gb.Model
		want  string
	}{
		{opts, gb.ModelDMG, "first gb"},
		{opts, gb.ModelCGB, "second gbc"},
		{Options{BootROMs: &BootROMSearch{Defaults: []string{empty}}}, gb.ModelDMG, ""},
		{Options{BootROM: "none", BootROMs: &BootROMSearch{Defaults: []string{first}}}, gb.ModelDMG, ""},
		// The folder chosen in the menu comes first.
		{Options{BootROMs: &BootROMSearch{Dir: second, Defaults: []string{first}}}, gb.ModelDMG, "second gb"},
		{Options{BootROMs: &BootROMSearch{Dir: empty, Defaults: []string{first}}}, gb.ModelDMG, "first gb"},
		{Options{BootROM: filepath.Join(second, "gbc_bios.bin")}, gb.ModelDMG, "second gbc"},
		{Options{BootROM: filepath.Join(empty, "missing.bin")}, gb.ModelDMG, ""},
	} {
		got, err := readBootROM(c.opts, c.model)
		if err != nil || string(got) != c.want {
			t.Errorf("readBootROM(%+v, %v) = %q, %v; want %q", c.opts, c.model, got, err, c.want)
		}
	}
}
