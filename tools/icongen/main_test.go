package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The icon files in the repository are those this program writes: after a
// change to the drawing, run go generate ./assets/icon.
func TestFilesUpToDate(t *testing.T) {
	files, err := Files()
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join("..", "..", "assets", "icon", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("assets/icon/%s is out of date: run go generate ./assets/icon", name)
		}
	}
}
