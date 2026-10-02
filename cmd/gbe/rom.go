package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxROMSize is the largest cartridge (MBC5, 8 MiB): a bigger entry in a zip
// archive is not a ROM, and reading it could exhaust the memory.
const maxROMSize = 8 << 20

// readROM reads a ROM file, or the first .gb or .gbc ROM of a zip archive,
// which is decompressed in memory. entry is the name of the ROM in the
// archive, empty for a plain ROM file.
func readROM(path string) (rom []byte, entry string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return data, "", nil
	}
	rom, entry, err = romFromZip(data)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	return rom, entry, nil
}

func romFromZip(data []byte) ([]byte, string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, "", err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !isROMName(f.Name) {
			continue
		}
		if f.UncompressedSize64 > maxROMSize {
			return nil, "", fmt.Errorf("%s: too large for a ROM (%d bytes)", f.Name, f.UncompressedSize64)
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
			return nil, "", fmt.Errorf("%s: too large for a ROM", f.Name)
		}
		return rom, f.Name, nil
	}
	return nil, "", fmt.Errorf("no .gb or .gbc ROM in the archive")
}

func isROMName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".gb" || ext == ".gbc"
}
