package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/rom"
)

// withLocks has g open the games as cmd/gbe does, with the locks in dir:
// two such Games play as two gbe would.
func withLocks(g *Game, dir string) *Game {
	g.open = func(path string, colorize bool) (*rom.Game, error) {
		return rom.Open(path, rom.Options{Model: gb.ModelAuto, BootROM: "none", Colorize: colorize, Strict: true, LockDir: dir})
	}
	return g
}

func TestSecondGbeOwnSaves(t *testing.T) {
	dir, locks := t.TempDir(), t.TempDir()
	zelda := writeRAMROM(t, filepath.Join(dir, "Zelda.gb"))
	first, second := withLocks(newDropGame(t), locks), withLocks(newDropGame(t), locks)
	first.drop(dropped(zelda))
	second.drop(dropped(zelda))
	if first.savePath != filepath.Join(dir, "Zelda.sav") || first.toast != first.tr().T("toast.game_loaded", "ZELDA") {
		t.Errorf("first: %s, toast %q", first.savePath, first.toast)
	}
	if second.savePath != filepath.Join(dir, "Zelda.2.sav") || second.statePath != filepath.Join(dir, "Zelda.2.state") ||
		second.toast != second.tr().T("toast.other_slot", "Zelda.2.sav") {
		t.Errorf("second: %s, %s, toast %q", second.savePath, second.statePath, second.toast)
	}

	// Each writes its own battery save.
	second.gb.RunFrame() // the game writes 0x77 to its RAM
	second.saveBattery()
	if _, err := os.Stat(filepath.Join(dir, "Zelda.sav")); err == nil {
		t.Error("the second gbe wrote the first one's save")
	}
	if data, err := os.ReadFile(filepath.Join(dir, "Zelda.2.sav")); err != nil || data[0] != 0x77 {
		t.Errorf("second save: %v", err)
	}

	// The first switches to another game: its slot is free again.
	first.switchGame(mustOpen(t, first, writeROM(t, filepath.Join(dir, "Mario.gb"), "MARIO", 0x03)))
	third := withLocks(newDropGame(t), locks)
	third.drop(dropped(zelda))
	if third.savePath != filepath.Join(dir, "Zelda.sav") {
		t.Errorf("third, once the first left: %s", third.savePath)
	}
}

// A dropped game that is not played gives its slot back.
func TestDroppedGameReleased(t *testing.T) {
	dir, locks := t.TempDir(), t.TempDir()
	zelda := writeROM(t, filepath.Join(dir, "Zelda.gb"), "ZELDA", 0x03)
	mario := writeROM(t, filepath.Join(dir, "Mario.gb"), "MARIO", 0x03)
	g := withLocks(newDropGame(t), locks)
	g.drop(dropped(zelda))
	g.drop(dropped(zelda)) // the same game: refused
	g.drop(dropped(mario)) // asked whether to switch...
	g.drop(dropped(mario)) // ...dropped again, replacing the first
	g.menu.keepPlaying(g)  // and kept out
	other := withLocks(newDropGame(t), locks)
	other.drop(dropped(mario))
	if other.savePath != filepath.Join(dir, "Mario.sav") {
		t.Errorf("Mario was not given back: %s", other.savePath)
	}
	g.releaseGame()
	other.drop(dropped(zelda)) // asks to switch
	if other.pending == nil || other.pending.Slot != 1 {
		t.Errorf("Zelda was not given back on closing: %+v", other.pending)
	}
}

func mustOpen(t *testing.T, g *Game, path string) *rom.Game {
	t.Helper()
	next, err := g.openGame(path)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

// writeRAMROM writes an MBC1 game with a battery that writes 0x77 at the
// start of its RAM, then loops.
func writeRAMROM(t *testing.T, path string) string {
	t.Helper()
	data := make([]byte, 0x8000)
	copy(data[0x134:], "ZELDA")
	data[0x147], data[0x149] = 0x03, 0x02
	copy(data[0x100:], []byte{
		0x3E, 0x0A, 0xEA, 0x00, 0x00, // LD A,0x0A; LD (0x0000),A: RAM on
		0x3E, 0x77, 0xEA, 0x00, 0xA0, // LD A,0x77; LD (0xA000),A
		0x18, 0xFE, // JR -2
	})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
