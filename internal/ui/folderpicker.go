package ui

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ncruces/zenity"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/rom"
)

// folderPicker asks the player for a folder.
type folderPicker interface {
	// pick opens the dialog, starting at start, without blocking: the
	// folder chosen ("" when canceled), or why the dialog could not open,
	// comes later on done.
	pick(title, start string, done chan<- folderPick)
}

// folderPick is the outcome of folderPicker.pick.
type folderPick struct {
	dir string
	err error
}

// systemPicker opens the folder dialog of the system: through osascript on
// macOS, the Win32 API on Windows, and zenity, qarma or matedialog on the
// other systems (an error when none is installed).
type systemPicker struct{}

func (systemPicker) pick(title, start string, done chan<- folderPick) {
	go func() {
		dir, err := zenity.SelectFile(zenity.Directory(), zenity.Title(title), zenity.Filename(start))
		if errors.Is(err, zenity.ErrCanceled) {
			dir, err = "", nil
		}
		done <- folderPick{dir, err}
	}()
}

// bootROMSearch is where the boot ROMs are searched, never nil.
func (g *Game) bootROMSearch() *rom.BootROMSearch {
	if g.bootROMs == nil {
		g.bootROMs = &rom.BootROMSearch{Dir: g.cfg.BootROMDir, Defaults: []string{"bios"}}
	}
	return g.bootROMs
}

// chooseBootROMDir opens the folder dialog for the boot ROMs, from the folder
// chosen before or the home folder. The game loop goes on: pollFolderPick
// applies the choice.
func (g *Game) chooseBootROMDir() {
	g.chooseFolder(g.cfg.BootROMDir, g.tr().T("boot_rom.dialog_title"), g.setBootROMDir)
}

// chooseGamesDir does the same for the games folder.
func (g *Game) chooseGamesDir() {
	g.chooseFolder(g.cfg.GamesDir, g.tr().T("games_dir.dialog_title"), g.setGamesDir)
}

// chooseFolder opens the folder dialog from start, or the home folder; set
// gets the folder chosen.
func (g *Game) chooseFolder(start, title string, set func(string)) {
	if g.picker == nil || g.picking {
		return
	}
	if start == "" {
		start, _ = os.UserHomeDir()
	}
	if start != "" {
		start += string(filepath.Separator) // open the folder itself
	}
	if g.picks == nil {
		g.picks = make(chan folderPick, 1)
	}
	g.picking, g.picked = true, set
	g.picker.pick(title, start, g.picks)
}

// pollFolderPick applies the folder chosen in the dialog, once it is closed.
func (g *Game) pollFolderPick() {
	select {
	case p := <-g.picks:
		g.picking = false
		switch {
		case p.err != nil:
			log.Printf("folder dialog: %v", p.err)
			g.notifyLong(g.tr().T("boot_rom.picker_failed"))
		case p.dir != "" && g.picked != nil:
			g.picked(p.dir)
		}
	default:
	}
}

// setBootROMDir makes dir the folder searched first for the boot ROMs ("" for
// none). It applies to the next game, or to the running one once reset.
func (g *Game) setBootROMDir(dir string) {
	if dir == g.cfg.BootROMDir {
		return
	}
	g.cfg.BootROMDir = dir
	g.bootROMSearch().Dir = dir
	g.saveConfig()
	if g.gb != nil {
		g.bootROMPending = true
	}
	if dir != "" && !hasBootROM(dir) {
		g.notifyLong(g.tr().T("boot_rom.empty"))
	}
}

// hasBootROM reports whether dir holds a boot ROM.
func hasBootROM(dir string) bool {
	for _, model := range []gb.Model{gb.ModelDMG, gb.ModelCGB} {
		if _, err := os.Stat(filepath.Join(dir, rom.BootROMName(model))); err == nil {
			return true
		}
	}
	return false
}

// shortDir is dir as the menu shows it: the home folder as ~, and ? for the
// characters the bitmap font cannot draw.
func shortDir(dir string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if dir == home {
			dir = "~"
		} else if rest, ok := strings.CutPrefix(dir, home+string(filepath.Separator)); ok {
			dir = "~" + string(filepath.Separator) + rest
		}
	}
	r := []rune(dir)
	for i, c := range r {
		if c < ' ' || c > '~' {
			r[i] = '?'
		}
	}
	return string(r)
}

// Scrolling of a text too long for its place: it shows its start for
// marqueePause ticks, then moves by one character every marqueeStep ticks,
// marqueeGap spaces separating its end from its start coming back.
const (
	marqueePause = 45
	marqueeStep  = 8
	marqueeGap   = 4
)

// marquee is the part of s shown at tick in width characters: s itself when
// it fits, else a window that scrolls through it and starts over.
func marquee(s string, width int, tick int64) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	loop := len(r) + marqueeGap // characters before the start comes back
	t := tick % int64(marqueePause+loop*marqueeStep)
	offset := 0
	if t >= marqueePause {
		offset = int(t-marqueePause) / marqueeStep
	}
	r = append(append(r, []rune(strings.Repeat(" ", marqueeGap))...), r...)
	return string(r[offset : offset+width])
}
