package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// gamesDir writes the games in a new folder, each a path relative to it,
// and returns the folder.
func gamesDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		writeROM(t, path, "GAME", 0x03)
	}
	return dir
}

// waitLibrary waits for the games folder to be read, and applies the list.
func waitLibrary(t *testing.T, g *Game) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for g.library.scanning {
		if time.Now().After(deadline) {
			t.Fatal("the games folder is never read")
		}
		time.Sleep(time.Millisecond)
		g.pollLibrary()
	}
}

// libraryNames lists the names of the games of the folder, with their
// subfolder.
func libraryNames(g *Game) string {
	var names []string
	for _, e := range g.library.games {
		name := e.name
		if e.sub != "" {
			name = filepath.ToSlash(e.sub) + ":" + name
		}
		names = append(names, name)
	}
	return strings.Join(names, ",")
}

// The games of the folder and of its subfolders are listed by name,
// whatever their case; the other files and the hidden ones are not.
func TestScanLibrary(t *testing.T) {
	dir := gamesDir(t, "tetris (World).gb", "GBC/Zelda DX (France).gbc", "GBC/Pokemon/Pokemon Jaune.zip",
		"Alleyway.GB", ".hidden/Secret.gb", ".Ghost.gb")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	g := newTestGame(t, newFakePads())
	g.library.games = scanLibrary(dir)
	want := "Alleyway,GBC/Pokemon:Pokemon Jaune,tetris (World),GBC:Zelda DX (France)"
	if got := libraryNames(g); got != want {
		t.Errorf("games %s, want %s", got, want)
	}
	if scanLibrary(filepath.Join(dir, "missing")) != nil {
		t.Error("a missing folder must hold no game")
	}
}

// A long name is cut, or scrolls while it is selected.
func TestListName(t *testing.T) {
	long := "Pokemon - Version Jaune - Edition Speciale Pikachu (France)"
	if got := listName("Tetris", gameNameWidth, true, 500); got != "Tetris" {
		t.Errorf("short name: %q", got)
	}
	if got := listName(long, gameNameWidth, false, 500); got != long[:gameNameWidth-3]+"..." {
		t.Errorf("long name: %q", got)
	}
	if got := listName(long, gameNameWidth, true, 0); got != long[:gameNameWidth] {
		t.Errorf("selected, at first: %q", got)
	}
	if got := listName(long, gameNameWidth, true, marqueePause+marqueeStep); got != long[1:gameNameWidth+1] {
		t.Errorf("selected, scrolling: %q", got)
	}
}

func TestNextWithLetter(t *testing.T) {
	var games []libraryGame
	for _, name := range []string{"Alleyway", "Asteroids", "Batman", "pinball", "Tetris"} {
		games = append(games, libraryGame{name: name})
	}
	for _, c := range []struct {
		from int
		c    rune
		want int
	}{
		{0, 'a', 1}, // the next one with A
		{1, 'a', 0}, // around the list
		{0, 'P', 3}, // whatever the case
		{4, 'b', 2},
		{0, 'z', -1},
	} {
		if got := nextWithLetter(games, c.from, c.c); got != c.want {
			t.Errorf("from %d, %c: %d, want %d", c.from, c.c, got, c.want)
		}
	}
}

// The games folder is chosen on the Folders page, then listed on the Games
// page, where they are launched.
func TestGamesFolder(t *testing.T) {
	g := newDropGame(t)
	picker := &fakePicker{}
	g.picker = picker
	m := &g.menu
	m.show()
	if !m.view(g).disabled[itemGames] {
		t.Fatal("Games... enabled without a recent game nor a games folder")
	}

	// Folders..., then Choose the folder.
	m.cursor = itemFolders
	g.actions = menuActions{ok: true}
	m.update(g)
	if v := m.view(g); m.page != pageFolders || m.tab != tabGamesDir || v.items[gamesDirCount] != "No folder chosen" ||
		!v.disabled[gamesDirForget] || v.footer != "Folder: none" {
		t.Fatalf("Folders page: page %d, tab %d, %q, footer %q", m.page, m.tab, v.items, v.footer)
	}
	m.update(g) // OK on Choose the folder
	dir := gamesDir(t, "Tetris.gb", "Alleyway.gb", "GBC/Zelda.gbc")
	g.picks <- folderPick{dir: dir}
	g.pollFolderPick()
	if g.cfg.GamesDir != dir || g.cfg.BootROMDir != "" {
		t.Fatalf("games folder %q, boot ROM folder %q", g.cfg.GamesDir, g.cfg.BootROMDir)
	}
	waitLibrary(t, g)
	if v := m.view(g); v.items[gamesDirCount] != "Games found: 3" || v.disabled[gamesDirForget] || g.toast != "" {
		t.Errorf("folder chosen: %q, toast %q", v.items, g.toast)
	}
	if saved, err := LoadConfig(g.cfgPath); err != nil || saved.GamesDir != dir {
		t.Errorf("saved games folder %q, %v", saved.GamesDir, err)
	}

	// Back, then Games...: without recent games, the All tab.
	g.actions = menuActions{back: true}
	m.update(g)
	if m.page != pageMain || m.cursor != itemFolders || m.view(g).disabled[itemGames] {
		t.Fatalf("Back: page %d, cursor %d", m.page, m.cursor)
	}
	m.cursor = itemGames
	g.actions = menuActions{ok: true}
	m.update(g)
	waitLibrary(t, g) // read again when the list opens
	v := m.view(g)
	if m.page != pageGames || m.tab != tabAll || strings.Join(v.items, ",") != "Alleyway,Tetris,Zelda" ||
		!v.tabs[0].disabled || v.tabs[1].label != "All (3)" || v.footer != "1/3" {
		t.Fatalf("Games page: page %d, tab %d, %q, tabs %+v, footer %q", m.page, m.tab, v.items, v.tabs, v.footer)
	}
	g.actions = menuActions{left: true} // no recent game: the tab stays
	m.update(g)
	if m.tab != tabAll {
		t.Errorf("Left without recent games: tab %d", m.tab)
	}
	g.actions = menuActions{down: true, downHeld: 1}
	m.update(g)
	m.update(g)
	if v := m.view(g); m.cursor != 2 || v.footer != "3/3   GBC/" {
		t.Errorf("Down twice: cursor %d, footer %q", m.cursor, v.footer)
	}

	// OK launches Zelda, which becomes a recent game; the list opens on it
	// again.
	g.actions = menuActions{ok: true}
	m.update(g)
	if g.gb == nil || g.romPath != filepath.Join(dir, "GBC", "Zelda.gbc") || m.open {
		t.Fatalf("launched %q, menu open %v", g.romPath, m.open)
	}
	m.show()
	m.cursor = itemGames
	m.update(g)
	if m.tab != tabRecent {
		t.Errorf("with a recent game, the Recent tab opens first: %d", m.tab)
	}
	g.actions = menuActions{right: true}
	m.update(g)
	if v := m.view(g); m.tab != tabAll || m.cursor != 2 || v.items[2] != "Zelda (playing)" ||
		!strings.HasSuffix(v.footer, "The game in progress will be saved") {
		t.Errorf("All tab again: tab %d, cursor %d, %q, footer %q", m.tab, m.cursor, v.items, v.footer)
	}

	// Forget the folder: nothing listed any more.
	m.page, m.tab, m.cursor = pageFolders, tabGamesDir, gamesDirForget
	g.actions = menuActions{ok: true}
	m.update(g)
	if g.cfg.GamesDir != "" || len(g.library.games) != 0 || m.cursor != gamesDirChoose {
		t.Errorf("Forget: folder %q, %d games, cursor %d", g.cfg.GamesDir, len(g.library.games), m.cursor)
	}
}

// A folder dropped on the Games tab of the Folders page becomes the games
// folder; one without a game is kept, with a warning.
func TestDropGamesFolder(t *testing.T) {
	g := newDropGame(t)
	g.menu = menu{open: true, page: pageFolders, tab: tabGamesDir}
	empty := t.TempDir()
	g.drop(dropped(empty))
	waitLibrary(t, g)
	if g.cfg.GamesDir != empty || g.cfg.BootROMDir != "" || g.toast != "No game in this folder" {
		t.Errorf("folder dropped: games %q, boot ROMs %q, toast %q", g.cfg.GamesDir, g.cfg.BootROMDir, g.toast)
	}
}

// The long list goes faster the longer Down is held, a page at a time, and
// to the games starting with a letter typed.
func TestGamesListMoves(t *testing.T) {
	g := newDropGame(t)
	var names []string
	for _, c := range "ABCDEFGHIJKLMNOPQRST" {
		names = append(names, string(c)+" game.gb", string(c)+" other.gb")
	}
	g.setGamesDir(gamesDir(t, names...))
	waitLibrary(t, g)
	m := &g.menu
	m.show()
	m.showGames(g, tabAll)
	m.rows = 10
	move := func(a menuActions, want int) {
		t.Helper()
		g.actions = a
		m.update(g)
		if m.cursor != want {
			t.Errorf("%+v: cursor %d, want %d", a, m.cursor, want)
		}
	}
	move(menuActions{pageDown: true}, 9)
	move(menuActions{pageDown: true}, 18)
	move(menuActions{pageUp: true}, 9)
	move(menuActions{pageUp: true}, 0)
	move(menuActions{letter: 'k'}, 20)
	move(menuActions{letter: 'k'}, 21) // the next one with K
	move(menuActions{letter: 'k'}, 20)
	move(menuActions{letter: 'z'}, 20) // none
	move(menuActions{up: true, upHeld: 1}, 19)
	move(menuActions{upHeld: 31}, 19)     // between two repeats
	move(menuActions{upHeld: 62}, 18)     // after a second: every other tick
	move(menuActions{upHeld: 63}, 18)     //
	move(menuActions{upHeld: 121}, 17)    // after two: every tick
	move(menuActions{pageDown: true}, 26) //
	move(menuActions{downHeld: 1, down: true}, 27)
	m.cursor = 39
	move(menuActions{down: true, downHeld: 1}, 0) // around the list

	// The view gives the games around the selected one, with the scroll
	// bar where they are.
	m.cursor = 30
	v := m.view(g)
	if v.total != 40 || v.offset != 0 || len(v.items) != 40 || v.selected != 30 {
		t.Errorf("view: offset %d of %d, %d items, selected %d", v.offset, v.total, len(v.items), v.selected)
	}
	l := layoutMenu(v, 0, 320, 288)
	// The selected game stays in the middle.
	if !l.above || !l.below || l.listFirst+l.selected != 30 || l.selected != len(l.items)/2 || l.listTotal != 40 {
		t.Errorf("layout: first %d, selected %d, above %v, below %v", l.listFirst, l.selected, l.above, l.below)
	}
}

// A list too long for a view gives only the games around the selected one.
func TestLongListWindow(t *testing.T) {
	g := newTestGame(t, newFakePads())
	for i := range 1000 {
		g.library.games = append(g.library.games, libraryGame{path: "/g/" + string(rune('a'+i%26)), name: "Game"})
	}
	m := &menu{open: true, page: pageGames, tab: tabAll, cursor: 500}
	v := m.view(g)
	if v.total != 1000 || len(v.items) != listWindow || v.offset != 500-listWindow/2 || v.offset+v.selected != 500 {
		t.Fatalf("view: offset %d of %d, %d items, selected %d", v.offset, v.total, len(v.items), v.selected)
	}
	for _, size := range [][2]float64{{160, 144}, {320, 288}, {1280, 1152}} {
		l := layoutMenu(v, 0, size[0], size[1])
		if l.listFirst+l.selected != 500 || !l.above || !l.below || l.y < 0 || l.y+l.h > size[1] {
			t.Errorf("%v: first %d, selected %d, panel %v+%v", size, l.listFirst, l.selected, l.y, l.h)
		}
	}
	m.cursor = 999
	if v := m.view(g); v.offset+len(v.items) != 1000 || v.offset+v.selected != 999 {
		t.Errorf("last game: offset %d, %d items, selected %d", v.offset, len(v.items), v.selected)
	}
}

// The drop screen offers the list of all the games under the recent ones,
// once the games folder has any; going back from it returns there.
func TestDropScreenAllGames(t *testing.T) {
	g := newDropGame(t)
	if g.dropEntries() != 0 {
		t.Fatal("entries without recent games nor games folder")
	}
	g.cfg.Recent = []RecentGame{{Path: "/r/a.gb"}}
	g.setGamesDir(gamesDir(t, "Tetris.gb", "Zelda.gb"))
	waitLibrary(t, g)
	if g.dropEntries() != 2 {
		t.Fatalf("%d entries, want the recent game and All games", g.dropEntries())
	}
	g.actions = menuActions{down: true}
	g.updateDropScreen()
	g.actions = menuActions{ok: true}
	g.updateDropScreen()
	if m := g.menu; !m.open || m.page != pageGames || m.tab != tabAll || !m.home {
		t.Fatalf("All games: %+v", m)
	}
	g.actions = menuActions{back: true}
	g.menu.update(g)
	if g.menu.open {
		t.Error("Back must return to the drop screen")
	}
}

func TestFastRepeatTick(t *testing.T) {
	var ticks []int
	for d := 1; d <= 130; d++ {
		if fastRepeatTick(d) {
			ticks = append(ticks, d)
		}
	}
	if !slices.Equal(ticks[:5], []int{1, 25, 30, 35, 40}) || !slices.Contains(ticks, 62) || slices.Contains(ticks, 63) ||
		!slices.Contains(ticks, 121) || !slices.Contains(ticks, 122) {
		t.Errorf("ticks %v", ticks)
	}
}

// A letter key is the letter of the active layout: Q on an AZERTY keyboard
// is the key of A on a QWERTY one.
func TestKeyLetter(t *testing.T) {
	defer func(f func(ebiten.Key) string) { keyName = f }(keyName)
	names := map[ebiten.Key]string{ebiten.KeyA: "q", ebiten.KeyDigit1: "&", ebiten.KeyM: ","}
	keyName = func(k ebiten.Key) string { return names[k] }
	for k, want := range map[ebiten.Key]rune{ebiten.KeyA: 'q', ebiten.KeyDigit1: '1', ebiten.KeyM: 'm', ebiten.KeyB: 'b'} {
		if got := keyLetter(k); got != want {
			t.Errorf("%v: %q, want %q", k, got, want)
		}
	}
}

func TestLoadConfigGamesDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.json")
	abs, err := filepath.Abs("games")
	if err != nil {
		t.Fatal(err)
	}
	for dir, want := range map[string]string{abs: abs, "games": ""} {
		cfg := DefaultConfig()
		cfg.GamesDir = dir
		if err := cfg.Save(path); err != nil {
			t.Fatal(err)
		}
		got, err := LoadConfig(path)
		if err != nil || got.GamesDir != want {
			t.Errorf("games_dir %q loaded as %q, %v; want %q", dir, got.GamesDir, err, want)
		}
	}
}

// A long name starts scrolling from its start, after the pause, as soon as
// it is highlighted; each list on its own, as the drop screen is drawn under
// the menu.
func TestMarqueeRestarts(t *testing.T) {
	g := newTestGame(t, newFakePads())
	now := int64(ebiten.Tick())
	g.marquees = map[string]marqueeStart{"games": {"1 0/g/a.gb", now - 500}}
	if got := g.marqueeTick("home", "0/g/c.gb"); got != 0 {
		t.Errorf("drop screen: tick %d, want 0", got)
	}
	if got := g.marqueeTick("games", "1 0/g/a.gb"); got != 500 {
		t.Errorf("same entry, after the drop screen was drawn: tick %d, want 500", got)
	}
	if got := g.marqueeTick("games", "1 1/g/b.gb"); got != 0 {
		t.Errorf("another entry: tick %d, want 0", got)
	}
}
