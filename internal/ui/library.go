package ui

import (
	"cmp"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/rom"
)

// The games folder (Config.GamesDir) lists the games it holds, those of its
// subfolders too, on the Games page of the menu and from the drop screen.
// It is read in the background, when gbe starts, when the folder changes
// and whenever the list is opened, so that a game added since shows up.

const (
	// maxLibrary is how many games the list holds at most.
	maxLibrary = 5000
	// gameNameWidth is the longest game name shown, in characters: a longer
	// one scrolls while it is selected, and is cut otherwise.
	gameNameWidth = 32
)

// libraryGame is a game of the games folder.
type libraryGame struct {
	path string // absolute
	name string // shown: the file name, without its extension
	sub  string // its folder, relative to the games folder ("" at the top)
}

// library is the list of the games folder.
type library struct {
	dir      string // the folder games was read from
	games    []libraryGame
	cursor   int // the game last selected, back to it when the list opens again
	scans    chan libraryScan
	scanning bool
	// announce warns the player if the folder just chosen holds no game,
	// once it has been read.
	announce bool
}

// libraryScan is what a scan found.
type libraryScan struct {
	dir   string
	games []libraryGame
}

// gameName is how the lists name the game at path: its file name, without
// the extension. The names of the usual collections tell the versions of a
// game apart, its title in the cartridge header does not.
func gameName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// scanLibrary lists the games of dir and of its subfolders, sorted by name.
// Hidden files and folders are left out, and the folders that cannot be
// read.
func scanLibrary(dir string) []libraryGame {
	var games []libraryGame
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil // unreadable: skipped
		case path != dir && strings.HasPrefix(d.Name(), "."):
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		case d.IsDir() || !rom.IsFileName(d.Name()):
			return nil
		}
		sub, _ := filepath.Rel(dir, filepath.Dir(path))
		if sub == "." {
			sub = ""
		}
		games = append(games, libraryGame{path: path, name: gameName(path), sub: sub})
		if len(games) >= maxLibrary {
			return filepath.SkipAll
		}
		return nil
	})
	slices.SortFunc(games, func(a, b libraryGame) int {
		return cmp.Or(strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)), strings.Compare(a.path, b.path))
	})
	return games
}

// refreshLibrary reads the games folder again, in the background:
// pollLibrary applies the list found. Without a folder, the list is empty.
func (g *Game) refreshLibrary() {
	lib := &g.library
	if g.cfg.GamesDir == "" {
		lib.dir, lib.games, lib.announce = "", nil, false
		return
	}
	if lib.scanning {
		return // pollLibrary starts again if the folder changed meanwhile
	}
	if lib.scans == nil {
		lib.scans = make(chan libraryScan, 1)
	}
	lib.scanning = true
	dir, scans := g.cfg.GamesDir, lib.scans
	go func() { scans <- libraryScan{dir, scanLibrary(dir)} }()
}

// pollLibrary applies the list of the games folder, once read. The selected
// game stays selected.
func (g *Game) pollLibrary() {
	lib := &g.library
	select {
	case s := <-lib.scans:
		lib.scanning = false
		if s.dir != g.cfg.GamesDir {
			g.refreshLibrary() // another folder was chosen meanwhile
			return
		}
		selected := ""
		if lib.cursor < len(lib.games) {
			selected = lib.games[lib.cursor].path
		}
		lib.dir, lib.games = s.dir, s.games
		lib.cursor = max(0, slices.IndexFunc(lib.games, func(e libraryGame) bool { return e.path == selected }))
		if lib.announce && len(lib.games) == 0 {
			g.notifyLong(g.tr().T("games_dir.empty"))
		}
		lib.announce = false
	default:
	}
}

// setGamesDir makes dir the games folder ("" for none), and reads it.
func (g *Game) setGamesDir(dir string) {
	if dir == g.cfg.GamesDir {
		return
	}
	g.cfg.GamesDir = dir
	g.saveConfig()
	g.library.games, g.library.cursor = nil, 0
	g.library.announce = dir != ""
	g.refreshLibrary()
}

// playLibrary starts the game i of the games folder, as playRecent does a
// recent game. A game that is gone has the folder read again.
func (g *Game) playLibrary(i int) {
	e := g.library.games[i]
	if g.gb != nil && sameFile(e.path, g.romPath) {
		g.notify(g.tr().T("toast.same_game"))
		return
	}
	next, err := g.openGame(e.path)
	if errors.Is(err, fs.ErrNotExist) {
		g.refreshLibrary()
		g.notifyLong(g.tr().T("error.recent_missing", e.name))
		return
	}
	if err == nil {
		g.switchGame(next)
	}
}

// savedAt is when the save state of the game at path was made, zero
// without one. The times are kept while a list is open: it asks for them on
// every frame.
func (g *Game) savedAt(path string) time.Time {
	if t, ok := g.stateTimes[path]; ok {
		return t
	}
	var t time.Time
	if fi, err := os.Stat(rom.Base(path) + ".state"); err == nil {
		t = fi.ModTime()
	}
	if g.stateTimes == nil {
		g.stateTimes = map[string]time.Time{}
	}
	g.stateTimes[path] = t
	return t
}

// marqueeStart is when the entry highlighted in a list became so.
type marqueeStart struct {
	key  string
	tick int64
}

// marqueeTick is the tick a scrolling name of list is at: it starts over,
// from its pause at the start, whenever another entry is highlighted. key
// tells the entry highlighted apart from the others. Each list keeps its
// own: the drop screen is drawn under the menu.
func (g *Game) marqueeTick(list, key string) int64 {
	now := int64(ebiten.Tick())
	if g.marquees == nil {
		g.marquees = map[string]marqueeStart{}
	}
	s, ok := g.marquees[list]
	if !ok || s.key != key {
		s = marqueeStart{key, now}
		g.marquees[list] = s
	}
	return now - s.tick
}

// listName is a game name as a list shows it: cut to width characters, or,
// while selected, scrolling through them.
func listName(name string, width int, selected bool, tick int64) string {
	r := []rune(name)
	switch {
	case len(r) <= width:
		return name
	case selected:
		return marquee(name, width, tick)
	}
	return string(r[:width-3]) + "..."
}

// nextWithLetter finds, after the game from, the next one whose name starts
// with c, going around the list; -1 if none does.
func nextWithLetter(games []libraryGame, from int, c rune) int {
	c = unicode.ToLower(c)
	for k := 1; k <= len(games); k++ {
		i := (from + k) % len(games)
		for _, first := range games[i].name {
			if unicode.ToLower(first) == c {
				return i
			}
			break
		}
	}
	return -1
}
