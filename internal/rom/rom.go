// Package rom opens a game: it reads the ROM, from a zip archive too, sets up
// its cartridge with the battery save, and builds the console to run it on.
package rom

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ldechoux/gbe/internal/gb"
)

// maxROMSize is the largest cartridge (MBC5, 8 MiB): a bigger entry in a zip
// archive is not a ROM, and reading it could exhaust the memory.
const maxROMSize = 8 << 20

var (
	// ErrNotROM is returned for a dropped file that is neither a ROM nor a
	// zip archive, judging by its extension.
	ErrNotROM = errors.New("not a .gb, .gbc or .zip file")
	// ErrNoROMInZip is returned for a zip archive without a .gb or .gbc file.
	ErrNoROMInZip = errors.New("no .gb or .gbc ROM in the archive")
	// ErrTooLarge is returned for a ROM larger than any cartridge.
	ErrTooLarge = errors.New("too large for a ROM")
)

// Read reads a ROM file, or the first .gb or .gbc ROM of a zip archive,
// which is decompressed in memory. entry is the name of the ROM in the
// archive, empty for a plain ROM file.
func Read(path string) (rom []byte, entry string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		if len(data) > maxROMSize {
			return nil, "", fmt.Errorf("%s: %w (%d bytes)", path, ErrTooLarge, len(data))
		}
		return data, "", nil
	}
	rom, entry, err = fromZip(data)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	return rom, entry, nil
}

func fromZip(data []byte) ([]byte, string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, "", err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !IsName(f.Name) {
			continue
		}
		if f.UncompressedSize64 > maxROMSize {
			return nil, "", fmt.Errorf("%s: %w (%d bytes)", f.Name, ErrTooLarge, f.UncompressedSize64)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, "", err
		}
		defer rc.Close()
		// The header size is not trusted: the reader stops past the limit.
		rom, err := io.ReadAll(io.LimitReader(rc, maxROMSize+1))
		if err != nil {
			return nil, "", fmt.Errorf("%s: %w", f.Name, err)
		}
		if len(rom) > maxROMSize {
			return nil, "", fmt.Errorf("%s: %w", f.Name, ErrTooLarge)
		}
		return rom, f.Name, nil
	}
	return nil, "", ErrNoROMInZip
}

// IsName reports whether name has the extension of a ROM, .gb or .gbc.
func IsName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".gb" || ext == ".gbc"
}

// IsFileName reports whether name has the extension of a file gbe opens: a
// ROM, or a zip archive holding one.
func IsFileName(name string) bool {
	return IsName(name) || strings.EqualFold(filepath.Ext(name), ".zip")
}

// Base is the ROM path without its .gb, .gbc or .zip extension, to which
// the save files extensions are appended: the saves of a zipped ROM sit next
// to the archive, which is never written.
func Base(path string) string {
	if IsFileName(path) {
		return strings.TrimSuffix(path, filepath.Ext(path))
	}
	return path
}

// Options are the settings a game is opened with.
type Options struct {
	// Model is the hardware: gb.ModelAuto runs each game on the one it was
	// made for.
	Model gb.Model
	// BootROM is the -bios flag: a boot ROM path, "none", or empty for
	// gb_bios.bin or gbc_bios.bin, in the first folder of BootROMs that has
	// it (ignored when none has).
	BootROM string
	// BootROMs are the folders searched for the default boot ROM, read each
	// time a console is made, so that a folder chosen in the menu applies to
	// the next reset; bios when nil.
	BootROMs *BootROMSearch
	// Colorize runs the DMG games on a Game Boy Color, which colorizes them,
	// when Model is auto.
	Colorize bool
	// Fresh ignores the save state when choosing the hardware: no window
	// will offer to resume it (headless mode).
	Fresh bool
	// Strict refuses the files whose extension is not .gb, .gbc or .zip
	// (ErrNotROM), as for a file dropped on the window.
	Strict bool
}

// Game is an opened game, ready to run.
type Game struct {
	Path    string // as given to Open
	Console *gb.GameBoy
	Title   string // from the cartridge header
	// SavePath and StatePath are the battery save and the save state, next
	// to the ROM.
	SavePath, StatePath string
	// AutoModel is true when the hardware follows the game (Options.Model
	// auto), so that the menu may switch it.
	AutoModel bool
	// NewConsole builds a console of the given model for this cartridge,
	// with its boot ROM.
	NewConsole func(gb.Model) (*gb.GameBoy, error)
}

// Open reads the game at path and builds its console. The battery save is
// loaded; left to auto, the console starts in the hardware mode of the save
// state, which could not be resumed otherwise (e.g. after colorizing DMG
// games).
func Open(path string, opts Options) (*Game, error) {
	if opts.Strict && !IsFileName(path) {
		return nil, fmt.Errorf("%s: %w", path, ErrNotROM)
	}
	data, entry, err := Read(path)
	if err != nil {
		return nil, err
	}
	if entry != "" {
		log.Printf("%s: %s", path, entry)
	}
	cart, err := gb.NewCartridge(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	preferred := gb.ResolveModel(cart, opts.Model)
	if opts.Model == gb.ModelAuto && !cart.ColorSupported() && opts.Colorize {
		preferred = gb.ModelCGB // a Game Boy Color colorizes DMG games (opt-in)
	}
	base := Base(path)
	g := &Game{
		Path:      path,
		Title:     cart.Title,
		SavePath:  base + ".sav",
		StatePath: base + ".state",
		AutoModel: opts.Model == gb.ModelAuto,
	}
	if cart.Battery {
		if data, err := os.ReadFile(g.SavePath); err == nil {
			cart.LoadSaveData(data)
		} else if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("reading save: %v", err)
		}
	}
	g.NewConsole = func(model gb.Model) (*gb.GameBoy, error) {
		boot, err := readBootROM(opts, model)
		if err != nil {
			return nil, err
		}
		return gb.NewModel(cart, boot, model)
	}

	// The menu goes to the preferred model when the player starts over or
	// resets.
	launch := preferred
	if data, err := os.ReadFile(g.StatePath); err == nil && opts.Model == gb.ModelAuto && !opts.Fresh {
		if m, err := gb.StateModel(data); err == nil && (m == gb.ModelDMG || m == gb.ModelCGB) {
			launch = m
		}
	}
	if g.Console, err = g.NewConsole(launch); err != nil {
		return nil, err
	}
	return g, nil
}

// BootROMSearch is where the default boot ROMs are searched: the folder
// chosen by the player first, then the default ones.
type BootROMSearch struct {
	Dir      string   // chosen in the menu; "" for none
	Defaults []string // the bios folders searched after it
}

// Dirs are the folders searched, in order.
func (s *BootROMSearch) Dirs() []string {
	if s == nil {
		return []string{"bios"}
	}
	if s.Dir == "" {
		return s.Defaults
	}
	return append([]string{s.Dir}, s.Defaults...)
}

// BootROMName is the file name of the boot ROM of model.
func BootROMName(model gb.Model) string {
	if model == gb.ModelCGB {
		return "gbc_bios.bin"
	}
	return "gb_bios.bin"
}

// Find returns the path of the boot ROM of model in the folders searched,
// or "" when none has it.
func (s *BootROMSearch) Find(model gb.Model) string {
	for _, dir := range s.Dirs() {
		path := filepath.Join(dir, BootROMName(model))
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			return path
		}
	}
	return ""
}

// readBootROM reads the boot ROM of model that opts asks for: nil when there
// is none.
func readBootROM(opts Options, model gb.Model) ([]byte, error) {
	switch opts.BootROM {
	case "none":
		return nil, nil
	case "":
		path := opts.BootROMs.Find(model)
		if path == "" {
			return nil, nil
		}
		return os.ReadFile(path)
	}
	boot, err := os.ReadFile(opts.BootROM)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return boot, err
}
