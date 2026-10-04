package ui

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/rom"
)

// droppedFS is what ebiten.DroppedFiles returns on desktops: the dropped
// files at its root, whose entries know their real path.
type droppedFS []droppedEntry

type droppedEntry struct {
	path string // real path
	dir  bool
}

func (e droppedEntry) Name() string               { return filepath.Base(e.path) }
func (e droppedEntry) IsDir() bool                { return e.dir }
func (e droppedEntry) Type() fs.FileMode          { return 0 }
func (e droppedEntry) Info() (fs.FileInfo, error) { return os.Stat(e.path) }
func (e droppedEntry) AbsPath() string            { return e.path }

var _ ebiten.AbsPather = droppedEntry{}

func (d droppedFS) Open(name string) (fs.File, error) { return nil, fs.ErrNotExist }

func (d droppedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	var out []fs.DirEntry
	for _, e := range d {
		out = append(out, e)
	}
	return out, nil
}

func dropped(paths ...string) droppedFS {
	var d droppedFS
	for _, p := range paths {
		fi, err := os.Stat(p)
		d = append(d, droppedEntry{path: p, dir: err == nil && fi.IsDir()})
	}
	return d
}

// writeROM writes a 32 KiB ROM titled title, of cartridge type typ (0x03:
// MBC1 with a battery), that loops forever.
func writeROM(t *testing.T, path, title string, typ byte) string {
	t.Helper()
	data := make([]byte, 0x8000)
	copy(data[0x134:], title)
	data[0x147], data[0x149] = typ, 0x02
	copy(data[0x100:], []byte{0x18, 0xFE}) // JR -2
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// newDropGame is a game without a ROM, which opens the dropped ones as
// cmd/gbe does.
func newDropGame(t *testing.T) *Game {
	t.Helper()
	g := newTestGame(t, newFakePads())
	g.stream = &audioStream{}
	g.open = func(path string, colorize bool) (*rom.Game, error) {
		return rom.Open(path, rom.Options{Model: gb.ModelAuto, BootROM: "none", Colorize: colorize, Strict: true})
	}
	return g
}

func TestDropWithoutGame(t *testing.T) {
	g := newDropGame(t)
	g.menu.show() // the Pause menu, opened from the drop screen
	dir := t.TempDir()
	g.drop(dropped(writeROM(t, filepath.Join(dir, "Zelda.gb"), "ZELDA", 0x03)))
	if g.gb == nil || g.title != "ZELDA" || !g.started || g.menu.open {
		t.Fatalf("dropped without a game: title %q, started %v, menu %v", g.title, g.started, g.menu.open)
	}
	if g.statePath != filepath.Join(dir, "Zelda.state") || g.savePath != filepath.Join(dir, "Zelda.sav") {
		t.Errorf("saves at %q and %q, want next to the ROM", g.statePath, g.savePath)
	}
	if g.toast != "Game started: ZELDA" {
		t.Errorf("toast %q", g.toast)
	}
}

func TestDropDuringGame(t *testing.T) {
	g := newDropGame(t)
	dir := t.TempDir()
	first := writeROM(t, filepath.Join(dir, "First.gb"), "FIRST", 0x03)
	g.drop(dropped(first))
	g.advance(false, false)
	g.rewind.record(g.gb)
	g.stream.push(make([]int16, 1600))
	old := g.gb

	// A second game: the player chooses.
	second := writeROM(t, filepath.Join(dir, "Second.gb"), "SECOND", 0x03)
	g.drop(dropped(filepath.Join(dir, "notes.txt"), second)) // the ROM is picked
	if g.gb != old || !g.menu.open || g.menu.page != pageSwitch || g.pending == nil {
		t.Fatalf("drop during a game: menu %+v, pending %v", g.menu, g.pending)
	}
	if title, items, footer := g.menu.lines(g); title != "NEW GAME" || items[0] != "Launch SECOND" ||
		items[1] != "Keep playing" || footer != "The game in progress will be saved" {
		t.Errorf("switch page: %q %q %q", title, items, footer)
	}

	// Escape keeps the game in progress.
	g.actions = menuActions{back: true}
	g.menu.update(g)
	if g.gb != old || g.menu.open || g.pending != nil {
		t.Error("Escape must keep playing")
	}

	// Launch: the game in progress is saved, the new one starts afresh.
	g.drop(dropped(second))
	g.actions = menuActions{ok: true} // the cursor is on Launch
	g.menu.update(g)
	if g.title != "SECOND" || g.gb == old || g.menu.open || g.pending != nil {
		t.Fatalf("launch: title %q, menu %v", g.title, g.menu.open)
	}
	if _, err := os.Stat(filepath.Join(dir, "First.state")); err != nil {
		t.Errorf("the game left is not saved: %v", err)
	}
	if g.rewind.n != 0 || g.stream.buffered() != 0 {
		t.Error("the rewind history and the audio of the game left are kept")
	}

	// Back to the first game: its save state is offered.
	g.drop(dropped(first))
	g.actions = menuActions{ok: true}
	g.menu.update(g)
	if g.title != "FIRST" || g.started || g.menu.page != pageStart || g.stateTime.IsZero() {
		t.Errorf("the save state of the first game is not offered: started %v, page %v", g.started, g.menu.page)
	}
}

// Dropped while the resume prompt waits, Keep playing goes back to it.
func TestDropDuringStartPage(t *testing.T) {
	g := newDropGame(t)
	dir := t.TempDir()
	path := writeROM(t, filepath.Join(dir, "First.gb"), "FIRST", 0x03)
	if err := os.WriteFile(filepath.Join(dir, "First.state"), []byte("state"), 0o644); err != nil {
		t.Fatal(err)
	}
	g.drop(dropped(path))
	if g.started || g.menu.page != pageStart {
		t.Fatal("the resume prompt must open")
	}
	g.drop(dropped(writeROM(t, filepath.Join(dir, "Second.gb"), "SECOND", 0x03)))
	if _, _, footer := g.menu.lines(g); footer != "" {
		t.Errorf("nothing is saved before the player chose: footer %q", footer)
	}
	g.actions = menuActions{toggle: true}
	g.menu.update(g)
	if !g.menu.open || g.menu.page != pageStart || g.title != "FIRST" {
		t.Errorf("Keep playing: page %v, title %q", g.menu.page, g.title)
	}
}

func TestDropSameGame(t *testing.T) {
	g := newDropGame(t)
	path := writeROM(t, filepath.Join(t.TempDir(), "Game.gb"), "GAME", 0x03)
	g.drop(dropped(path))
	old := g.gb
	g.drop(dropped(path))
	if g.gb != old || g.menu.open || g.toast != "This game is already running" {
		t.Errorf("same game: menu %v, toast %q", g.menu.open, g.toast)
	}
}

func TestDropErrors(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "docs.zip")
	if err := os.WriteFile(zipPath, []byte("PK\x03\x04 not really a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{"text file", writeROM(t, filepath.Join(dir, "notes.txt"), "TXT", 0), "Unsupported file: notes.txt (.gb, .gbc or .zip)"},
		{"folder", dir, "Drop a file, not a folder"},
		{"tiny", writeFile(t, filepath.Join(dir, "tiny.gb"), "tiny"), "This file is not a Game Boy ROM"},
		{"unsupported", writeROM(t, filepath.Join(dir, "huc.gbc"), "HUC", 0x22), "Unsupported cartridge (type 0x22)"},
		{"bad zip", zipPath, "Cannot read docs.zip"},
	} {
		g := newDropGame(t)
		g.drop(dropped(tc.path))
		if g.gb != nil || g.toast != tc.want {
			t.Errorf("%s: toast %q, want %q", tc.name, g.toast, tc.want)
		}
		if g.toastUntil-ebiten.Tick() != dropToastTicks {
			t.Errorf("%s: shown %d ticks", tc.name, g.toastUntil-ebiten.Tick())
		}
	}
	g := newDropGame(t)
	g.cfg.Language = "fr"
	g.drop(dropped(filepath.Join(dir, "huc.gbc")))
	if g.toast != "Cartouche non supportee (type 0x22)" {
		t.Errorf("French: %q", g.toast)
	}
}

func writeFile(t *testing.T, path, data string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Without a game, the menu works, with the game entries greyed out and
// skipped, and Update does not emulate anything.
func TestNoGame(t *testing.T) {
	g := newDropGame(t)
	g.closeGame()
	g.keepPrevious()
	g.saveBattery()
	m := &g.menu
	m.show()
	v := m.view(g)
	for _, i := range []int{itemSaveState, itemLoadState, itemReset} {
		if !v.disabled[i] {
			t.Errorf("entry %q enabled without a game", v.items[i])
		}
	}
	m.cursor = itemLanguage
	g.actions = menuActions{down: true}
	m.update(g)
	if m.cursor != itemQuit {
		t.Errorf("Down from Language: cursor %d, want Quit", m.cursor)
	}
	g.actions = menuActions{up: true}
	m.update(g)
	if m.cursor != itemLanguage {
		t.Errorf("Up from Quit: cursor %d, want Language", m.cursor)
	}
	m.cursor = itemReset
	g.actions = menuActions{ok: true}
	m.update(g)
	if !m.open || g.gb != nil {
		t.Error("a greyed out entry did something")
	}
	m.open = false
	if err := g.Update(); err != nil || g.gb != nil {
		t.Errorf("Update without a game: %v", err)
	}
}

// startGame leaves nothing of the previous game: the rewind history, the
// audio, the speed indicator.
func TestStartGameResets(t *testing.T) {
	g := newFrameGame(t)
	g.advance(true, false)
	g.stream.push(make([]int16, 1600))
	next, err := rom.Open(writeROM(t, filepath.Join(t.TempDir(), "Next.gb"), "NEXT", 0x03),
		rom.Options{BootROM: "none"})
	if err != nil {
		t.Fatal(err)
	}
	g.startGame(next)
	if g.rewind.n != 0 || g.stream.buffered() != 0 || g.speed != playNormal || g.frame != 0 {
		t.Errorf("rewind %d, audio %d, speed %v", g.rewind.n, g.stream.buffered(), g.speed)
	}
	if g.prevShades != *g.gb.Framebuffer() || !g.stateTime.Equal(time.Time{}) {
		t.Error("the previous frame or the save state time belong to the game left")
	}
}

func TestRecentGames(t *testing.T) {
	g := newDropGame(t)
	dir := t.TempDir()
	var paths []string
	for _, name := range []string{"A", "B", "C", "D", "E", "F"} {
		paths = append(paths, writeROM(t, filepath.Join(dir, name+".gb"), "GAME "+name, 0x03))
	}
	g.drop(dropped(paths[0]))
	for _, p := range paths[1:] {
		g.drop(dropped(p))
		g.actions = menuActions{ok: true} // Launch
		g.menu.update(g)
	}
	g.drop(dropped(paths[2])) // C again: back to the top
	g.actions = menuActions{ok: true}
	g.menu.update(g)
	var titles []string
	for _, r := range g.cfg.Recent {
		titles = append(titles, r.Title)
	}
	if want := "GAME C,GAME F,GAME E,GAME D,GAME B"; strings.Join(titles, ",") != want {
		t.Errorf("recent %v, want %s", titles, want)
	}

	// The next launch, without a ROM, offers them; Down and OK launch the
	// second one.
	cfg, err := LoadConfig(g.cfgPath)
	if err != nil || len(cfg.Recent) != maxRecent || cfg.Recent[0].Path != paths[2] {
		t.Fatalf("saved recent games: %v %+v", err, cfg.Recent)
	}
	next := newDropGame(t)
	next.cfg, next.cfgPath = cfg, g.cfgPath
	next.actions = menuActions{down: true}
	next.updateDropScreen()
	next.actions = menuActions{ok: true}
	next.updateDropScreen()
	if next.title != "GAME F" || next.cfg.Recent[0].Title != "GAME F" {
		t.Errorf("launched %q, recent %+v", next.title, next.cfg.Recent)
	}
}

func TestRecentGameMissing(t *testing.T) {
	g := newDropGame(t)
	dir := t.TempDir()
	kept := writeROM(t, filepath.Join(dir, "Kept.gb"), "KEPT", 0x03)
	gone := filepath.Join(dir, "Gone.gb")
	g.cfg.Recent = []RecentGame{{Path: gone, Title: "GONE"}, {Path: kept, Title: "KEPT"}}
	g.actions = menuActions{ok: true}
	g.updateDropScreen()
	if g.gb != nil || len(g.cfg.Recent) != 1 || g.toast != "Not found, removed from the list: GONE" {
		t.Errorf("missing game: recent %+v, toast %q", g.cfg.Recent, g.toast)
	}

	g.cfg.Recent = append(g.cfg.Recent, RecentGame{Path: gone, Title: "GONE"})
	g.pruneRecent()
	if cfg, _ := LoadConfig(g.cfgPath); len(cfg.Recent) != 1 || cfg.Recent[0].Title != "KEPT" {
		t.Errorf("pruned: %+v", cfg.Recent)
	}
}

func TestRecentName(t *testing.T) {
	for _, tc := range []struct {
		r    RecentGame
		want string
	}{
		{RecentGame{Path: "/roms/Tetris.gb", Title: "TETRIS"}, "TETRIS"},
		{RecentGame{Path: "/roms/Homebrew.gbc"}, "Homebrew"},
		{RecentGame{Path: "/roms/A very long file name for a game.zip"}, "A very long file name for..."},
	} {
		if got := recentName(tc.r); got != tc.want {
			t.Errorf("recentName(%+v) = %q, want %q", tc.r, got, tc.want)
		}
	}
}

func TestArrowBounce(t *testing.T) {
	var got []int
	for tick := int64(0); tick < 5*arrowTicks; tick += arrowTicks {
		got = append(got, arrowBounce(tick))
	}
	if want := []int{0, 1, 2, 1, 0}; !slices.Equal(got, want) {
		t.Errorf("arrow heights %v, want %v", got, want)
	}
}

func TestRecentPage(t *testing.T) {
	g := newDropGame(t)
	m := &g.menu
	m.show()
	if !m.view(g).disabled[itemRecent] {
		t.Error("Recent games enabled without any")
	}

	dir := t.TempDir()
	a := writeROM(t, filepath.Join(dir, "A.gb"), "GAME A", 0x03)
	b := writeROM(t, filepath.Join(dir, "B.gb"), "GAME B", 0x03)
	g.drop(dropped(b))
	g.drop(dropped(a))
	g.actions = menuActions{ok: true}
	m.update(g) // Launch A: recent is A, B
	m.show()
	m.cursor = itemRecent
	m.update(g) // OK opens the page
	if m.page != pageRecent {
		t.Fatalf("page %v, want Recent games", m.page)
	}
	if title, items, footer := m.lines(g); title != "RECENT GAMES" ||
		strings.Join(items, ",") != "GAME A (playing),GAME B,Back" || footer != "The game in progress will be saved" {
		t.Errorf("recent page: %q %q %q", title, items, footer)
	}

	m.update(g) // OK on the game in progress
	if g.title != "GAME A" || !m.open || g.toast != "This game is already running" {
		t.Errorf("same game: title %q, toast %q", g.title, g.toast)
	}
	g.actions = menuActions{down: true}
	m.update(g)
	g.actions = menuActions{ok: true}
	m.update(g) // GAME B
	// B was saved when A was launched: its save state is offered.
	if g.title != "GAME B" || m.page != pageStart || g.cfg.Recent[0].Title != "GAME B" {
		t.Errorf("launch B: title %q, page %v", g.title, m.page)
	}
	g.actions = menuActions{ok: true}
	m.update(g) // Resume
	if _, err := os.Stat(filepath.Join(dir, "A.state")); err != nil {
		t.Errorf("game A is not saved: %v", err)
	}

	m.show()
	m.cursor = itemRecent
	m.update(g)
	g.actions = menuActions{back: true}
	m.update(g)
	if m.page != pageMain || m.cursor != itemRecent {
		t.Errorf("Back: page %v, cursor %d", m.page, m.cursor)
	}
}
