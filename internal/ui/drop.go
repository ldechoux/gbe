package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/i18n"
	"github.com/ldechoux/gbe/internal/menusound"
	"github.com/ldechoux/gbe/internal/rom"
)

// dropToastTicks is how long an error about a dropped file stays on screen:
// longer than the other notifications, which are shorter to read.
const dropToastTicks = 4 * 60

// drop opens the game dropped on the window: the first file with the
// extension of a ROM or of a zip archive, or else the first one, to say why
// it cannot be opened. Without a game, it starts at once; during one, the
// player chooses between the two (pageSwitch).
func (g *Game) drop(fsys fs.FS) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil || len(entries) == 0 {
		log.Printf("reading the dropped files: %v", err)
		return
	}
	e := entries[0]
	for _, c := range entries {
		if !c.IsDir() && rom.IsFileName(c.Name()) {
			e = c
			break
		}
	}
	l := g.tr()
	path := e.Name()
	if p, ok := e.(ebiten.AbsPather); ok {
		path = p.AbsPath()
	}
	if e.IsDir() {
		// On the Boot ROM page, a folder becomes the one of the boot ROMs:
		// the way to choose it without a folder dialog.
		if g.menu.open && g.menu.page == pageBootROM && filepath.IsAbs(path) {
			g.menuSound(menusound.Change)
			g.setBootROMDir(path)
			return
		}
		g.notifyLong(l.T("error.folder"))
		return
	}
	next, err := g.openGame(path)
	if err != nil {
		return
	}
	switch {
	case g.gb == nil:
		g.switchGame(next)
	case sameFile(next.Path, g.romPath):
		g.notify(l.T("toast.same_game"))
	default:
		g.pending = next
		g.menu.showSwitch()
	}
}

// openGame opens the game at path with the settings of the menu. On
// failure, a notification explains why.
func (g *Game) openGame(path string) (*rom.Game, error) {
	if g.open == nil {
		return nil, errors.New("no way to open a game")
	}
	next, err := g.open(path, g.cfg.ColorizeDMG)
	if err != nil {
		log.Printf("opening a game: %v", err) // err names the file
		g.notifyLong(dropError(g.tr(), filepath.Base(path), err))
	}
	return next, err
}

// dropError explains, in language l, why the dropped file name cannot be
// played.
func dropError(l *i18n.Locale, name string, err error) string {
	var typ gb.CartridgeTypeError
	switch {
	case errors.Is(err, rom.ErrNotROM):
		return l.T("error.not_rom_ext", name)
	case errors.Is(err, rom.ErrNoROMInZip):
		return l.T("error.zip_empty")
	case errors.Is(err, rom.ErrTooLarge), errors.Is(err, gb.ErrROMTooSmall):
		return l.T("error.not_gb")
	case errors.As(err, &typ):
		return l.T("error.unsupported_cart", fmt.Sprintf("0x%02X", byte(typ)))
	}
	return l.T("error.unreadable", name)
}

// sameFile reports whether a and b are the same file.
func sameFile(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	fa, errA := os.Stat(a)
	fb, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return os.SameFile(fa, fb)
}

// switchGame leaves the current game, saved as when quitting so that it can
// be resumed later, and starts next.
func (g *Game) switchGame(next *rom.Game) {
	g.closeGame()
	g.startGame(next)
	g.notify(g.tr().T("toast.game_loaded", next.Title))
}

// closeGame saves the current game, if any: its state, unless the resume
// prompt is still waiting for a choice, and its battery RAM.
func (g *Game) closeGame() {
	if g.gb == nil {
		return
	}
	if g.started {
		g.saveState()
	}
	g.saveBattery()
}

// startGame runs next, from scratch: nothing of the previous game is kept.
// When next has a save state, the player is asked whether to resume it.
func (g *Game) startGame(next *rom.Game) {
	g.gb, g.title, g.romPath = next.Console, next.Title, next.Path
	g.savePath, g.statePath = next.SavePath, next.StatePath
	g.autoModel, g.newConsole = next.AutoModel, next.NewConsole
	g.pending, g.bootROMPending = nil, false
	g.addRecent(next)

	g.rewind.clear()
	g.rewindTick, g.speed, g.frame = 0, playNormal, 0
	if g.pads != nil {
		g.rumble.stop(g.pads)
	}
	if g.stream != nil {
		g.stream.reset()
	}
	if g.fx != nil {
		g.fx.Reset()
	}
	g.keepPrevious()

	g.stateTime, g.started = time.Time{}, true
	g.menu = menu{}
	if fi, err := os.Stat(g.statePath); g.statePath != "" && err == nil {
		g.stateTime, g.started = fi.ModTime(), false
		g.menu.showStart()
	}
	ebiten.SetWindowTitle(windowTitle(g.tr(), g.title, 0, !g.started))
}

// notifyLong shows msg for dropToastTicks.
func (g *Game) notifyLong(msg string) {
	g.notify(msg)
	g.toastUntil = ebiten.Tick() + dropToastTicks
}
