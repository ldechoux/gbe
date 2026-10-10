package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/rom"
)

// fakePicker records the dialogs opened; the test answers on Game.picks.
type fakePicker struct{ starts []string }

func (f *fakePicker) pick(title, start string, done chan<- folderPick) {
	f.starts = append(f.starts, start)
}

// bootROMDir writes the boot ROMs of models in a new folder.
func bootROMDir(t *testing.T, models ...gb.Model) string {
	t.Helper()
	dir := t.TempDir()
	for _, m := range models {
		if err := os.WriteFile(filepath.Join(dir, rom.BootROMName(m)), make([]byte, 0x100), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestBootROMPage(t *testing.T) {
	g := newTestGame(t, newFakePads())
	picker := &fakePicker{}
	g.picker = picker
	g.bootROMs = &rom.BootROMSearch{Defaults: []string{t.TempDir()}}
	m := &g.menu
	m.show()
	m.cursor = itemFolders
	g.actions = menuActions{ok: true}
	m.update(g)
	if m.page != pageFolders || m.tab != tabGamesDir {
		t.Fatalf("OK on Folders...: page %d, tab %d", m.page, m.tab)
	}
	g.actions = menuActions{right: true} // the Boot ROM tab
	m.update(g)
	if m.tab != tabBootROM || m.cursor != bootChoose {
		t.Fatalf("Right: tab %d, cursor %d", m.tab, m.cursor)
	}
	v := m.view(g)
	if v.footer != "Folder: automatic search" || !v.disabled[bootAuto] || !v.disabled[bootGB] {
		t.Fatalf("no folder chosen: footer %q, disabled %v", v.footer, v.disabled)
	}
	if v.items[bootGB] != "Game Boy        missing" {
		t.Errorf("GB line %q", v.items[bootGB])
	}
	g.actions = menuActions{down: true}
	m.update(g)
	if m.cursor != bootBack {
		t.Fatalf("Down skips the greyed-out entries: cursor %d, want Back", m.cursor)
	}

	// The dialog opens from the home folder; the folder chosen applies once
	// it is closed.
	m.cursor = bootChoose
	g.actions = menuActions{ok: true}
	m.update(g)
	m.update(g) // a second OK while the dialog is open does nothing
	home, _ := os.UserHomeDir()
	if len(picker.starts) != 1 || picker.starts[0] != home+string(filepath.Separator) || !g.picking {
		t.Fatalf("dialogs opened from %q, picking %v", picker.starts, g.picking)
	}
	dir := bootROMDir(t, gb.ModelDMG)
	g.picks <- folderPick{dir: dir}
	g.pollFolderPick()
	if g.picking || g.cfg.BootROMDir != dir || g.bootROMs.Dir != dir || g.toast != "" {
		t.Fatalf("after the choice: picking %v, config %q, search %q, toast %q", g.picking, g.cfg.BootROMDir, g.bootROMs.Dir, g.toast)
	}
	saved, err := LoadConfig(g.cfgPath)
	if err != nil || saved.BootROMDir != dir {
		t.Fatalf("saved folder %q, %v", saved.BootROMDir, err)
	}
	v = m.view(g)
	if v.items[bootGB] != "Game Boy        found" || v.items[bootGBC] != "Game Boy Color  missing" || v.disabled[bootAuto] {
		t.Errorf("folder with the GB boot ROM: %q, %q, disabled %v", v.items[bootGB], v.items[bootGBC], v.disabled)
	}
	if !strings.HasPrefix(v.footer, "Folder: ") || g.bootROMPending {
		t.Errorf("footer %q, pending %v without a game", v.footer, g.bootROMPending)
	}

	// Canceled: nothing changes. The dialog failed: the drop is suggested.
	for _, c := range []struct {
		pick folderPick
		want string
	}{
		{folderPick{}, ""},
		{folderPick{err: errors.New("zenity: not found")}, "No folder dialog: drop the folder on the window"},
	} {
		g.toast = ""
		g.chooseBootROMDir()
		g.picks <- c.pick
		g.pollFolderPick()
		if g.cfg.BootROMDir != dir || g.toast != c.want {
			t.Errorf("%+v: folder %q, toast %q, want %q", c.pick, g.cfg.BootROMDir, g.toast, c.want)
		}
	}

	// A folder without a boot ROM is kept, with a warning.
	empty := t.TempDir()
	g.setBootROMDir(empty)
	if g.cfg.BootROMDir != empty || g.toast != "No boot ROM in this folder" {
		t.Errorf("empty folder: %q, toast %q", g.cfg.BootROMDir, g.toast)
	}

	// Automatic search forgets the folder.
	m.cursor = bootAuto
	g.actions = menuActions{ok: true}
	m.update(g)
	if g.cfg.BootROMDir != "" || g.bootROMs.Dir != "" || m.cursor != bootChoose {
		t.Errorf("Automatic search: folder %q, search %q, cursor %d", g.cfg.BootROMDir, g.bootROMs.Dir, m.cursor)
	}
	g.actions = menuActions{back: true}
	m.update(g)
	if m.page != pageMain || m.cursor != itemFolders {
		t.Errorf("Back: page %d, cursor %d", m.page, m.cursor)
	}
}

// A folder dropped on the Boot ROM tab becomes the folder of the boot
// ROMs; elsewhere, it is refused.
func TestDropBootROMFolder(t *testing.T) {
	dir := bootROMDir(t, gb.ModelDMG, gb.ModelCGB)
	g := newDropGame(t)
	g.drop(dropped(dir))
	if g.cfg.BootROMDir != "" || g.toast != "Drop a file, not a folder" {
		t.Fatalf("folder dropped outside the page: %q, toast %q", g.cfg.BootROMDir, g.toast)
	}
	g.toast = ""
	g.menu = menu{open: true, page: pageFolders, tab: tabBootROM}
	g.drop(dropped(dir))
	if g.cfg.BootROMDir != dir || g.toast != "" {
		t.Fatalf("folder dropped on the page: %q, toast %q", g.cfg.BootROMDir, g.toast)
	}
}

// During a game, a new folder waits for a reset, which makes a new console
// of the same model with the boot ROM.
func TestBootROMFolderReset(t *testing.T) {
	g := newDropGame(t)
	g.bootROMs = &rom.BootROMSearch{}
	g.open = func(path string, colorize bool) (*rom.Game, error) {
		return rom.Open(path, rom.Options{Model: gb.ModelAuto, BootROMs: g.bootROMs, Colorize: colorize})
	}
	g.drop(dropped(writeROM(t, filepath.Join(t.TempDir(), "game.gb"), "GAME", 0)))
	if g.gb == nil || g.gb.BootROMActive() {
		t.Fatal("the game must start without a boot ROM")
	}
	g.setBootROMDir(bootROMDir(t, gb.ModelDMG))
	if !g.bootROMPending || mainFooter(g) != "Reset to apply the settings" {
		t.Fatalf("pending %v, footer %q", g.bootROMPending, mainFooter(g))
	}
	g.restart()
	if g.bootROMPending || !g.gb.BootROMActive() || g.gb.IsCGB() {
		t.Errorf("after the reset: pending %v, boot ROM %v, CGB %v", g.bootROMPending, g.gb.BootROMActive(), g.gb.IsCGB())
	}
}

func TestShortDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	sep := string(filepath.Separator)
	for _, c := range []struct{ dir, want string }{
		{home, "~"},
		{home + sep + "bios", "~" + sep + "bios"},
		{sep + "Jeux" + sep + "Bios é", sep + "Jeux" + sep + "Bios ?"},
	} {
		if got := shortDir(c.dir); got != c.want {
			t.Errorf("shortDir(%q) = %q, want %q", c.dir, got, c.want)
		}
	}
}

// A folder too long for the page shows its start, then scrolls one
// character at a time and starts over.
func TestMarquee(t *testing.T) {
	if got := marquee("/bios", 10, 500); got != "/bios" {
		t.Errorf("short text: %q", got)
	}
	const s = "/Users/me/Games/bios" // 20 characters in 10
	for _, c := range []struct {
		tick int64
		want string
	}{
		{0, "/Users/me/"},
		{marqueePause - 1, "/Users/me/"},
		{marqueePause + marqueeStep, "Users/me/G"},
		{marqueePause + 15*marqueeStep, "/bios    /"},
		{marqueePause + 24*marqueeStep, "/Users/me/"},                    // the start is back
		{marqueePause + 24*marqueeStep + marqueePause - 1, "/Users/me/"}, // and pauses again
	} {
		if got := marquee(s, 10, c.tick); got != c.want {
			t.Errorf("tick %d: %q, want %q", c.tick, got, c.want)
		}
	}
}

func TestLoadConfigBootROMDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.json")
	abs, err := filepath.Abs("bios")
	if err != nil {
		t.Fatal(err)
	}
	for dir, want := range map[string]string{abs: abs, "bios": ""} {
		cfg := DefaultConfig()
		cfg.BootROMDir = dir
		if err := cfg.Save(path); err != nil {
			t.Fatal(err)
		}
		got, err := LoadConfig(path)
		if err != nil || got.BootROMDir != want {
			t.Errorf("boot_rom_dir %q loaded as %q, %v; want %q", dir, got.BootROMDir, err, want)
		}
	}
}
