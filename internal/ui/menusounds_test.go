package ui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/menusound"
)

// soundLog records the menu sounds asked for.
type soundLog struct{ kinds []menusound.Kind }

func (s *soundLog) play(k menusound.Kind, volume float64) { s.kinds = append(s.kinds, k) }

// heard returns the sounds played since the last call.
func (s *soundLog) heard() []menusound.Kind {
	k := s.kinds
	s.kinds = nil
	return k
}

func TestMenuSounds(t *testing.T) {
	g := newTestGame(t, newFakePads())
	log := &soundLog{}
	g.sfx = log
	m := &g.menu
	do := func(what string, a menuActions, want ...menusound.Kind) {
		t.Helper()
		g.actions = a
		m.update(g)
		if got := log.heard(); !slices.Equal(got, want) {
			t.Errorf("%s: sounds %v, want %v", what, got, want)
		}
	}
	const (
		move, change, enter, back, refuse = menusound.Move, menusound.Change, menusound.Enter,
			menusound.Back, menusound.Refuse
	)

	m.show()
	do("Down", menuActions{down: true}, move)
	if m.cursor != itemDisplay {
		t.Fatalf("Down from Resume with no recent game: cursor %d, want Display", m.cursor)
	}
	m.cursor = itemRecent
	do("OK on a greyed-out entry", menuActions{ok: true}, refuse)
	m.cursor = itemResume
	do("Right on Resume", menuActions{right: true})
	m.cursor = itemSpeed
	do("Right on Fast forward", menuActions{right: true}, change)

	m.cursor = itemDisplay
	do("OK on Display...", menuActions{ok: true}, enter)
	m.cursor = displayPalette
	do("Left on Palette", menuActions{left: true}, change)
	g.cfg.Scale = maxScale
	m.cursor = displayScale
	do("Right on the largest scale", menuActions{right: true}, refuse)
	m.cursor = displayBack
	do("OK on Back", menuActions{ok: true}, back)
	do("Esc on the main page", menuActions{back: true}, back)
	if m.open {
		t.Fatal("the menu stays open")
	}

	// The sounds can be turned off on the sound page: turning them off is
	// silent, turning them back on is heard.
	*m = menu{open: true, page: pageSound, cursor: soundMenuSounds}
	do("UI sounds off", menuActions{right: true})
	do("Up with the sounds off", menuActions{up: true})
	m.cursor = soundMenuSounds
	do("UI sounds on", menuActions{ok: true}, change)
	if !g.cfg.MenuSounds {
		t.Fatal("UI sounds still off")
	}
	if cfg, _ := LoadConfig(g.cfgPath); !cfg.MenuSounds {
		t.Error("UI sounds not saved")
	}
	m.cursor = soundVolume
	do("OK on Volume", menuActions{ok: true})
	g.cfg.Volume = 1
	do("Right on Volume at 100%", menuActions{right: true}, refuse)

	*m = menu{open: true, page: pageControls, cursor: 0}
	do("OK on a key", menuActions{ok: true}, enter)
	if !m.capturing {
		t.Fatal("not capturing")
	}
	m.captureKey(g, Hotkey{Key: ebiten.KeyK})
	if got := log.heard(); !slices.Equal(got, []menusound.Kind{change}) {
		t.Errorf("a key bound: sounds %v, want change", got)
	}
	m.capturing = true
	m.captureKey(g, Hotkey{Key: ebiten.KeyEscape})
	if got := log.heard(); !slices.Equal(got, []menusound.Kind{back}) {
		t.Errorf("Esc while capturing: sounds %v, want back", got)
	}

	*m = menu{open: true, page: pageStart}
	do("Esc on the start page, which waits for a choice", menuActions{back: true})
	do("Resume on the start page", menuActions{ok: true}, enter)
}

// The list of recent games on the drop screen moves with the Move sound.
func TestDropScreenSounds(t *testing.T) {
	g := newTestGame(t, newFakePads())
	log := &soundLog{}
	g.sfx = log
	g.cfg.Recent = []RecentGame{{Path: "/a.gb"}, {Path: "/b.gb"}}
	g.actions = menuActions{down: true}
	g.updateDropScreen()
	g.actions = menuActions{up: true}
	g.updateDropScreen()
	if got := log.heard(); !slices.Equal(got, []menusound.Kind{menusound.Move, menusound.Move}) {
		t.Errorf("drop screen: sounds %v", got)
	}
}

func TestMenuSoundsConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if cfg, _ := LoadConfig(path); !cfg.MenuSounds {
		t.Error("menu sounds off by default")
	}
	os.WriteFile(path, []byte(`{"menu_sounds": false}`), 0o644)
	if cfg, _ := LoadConfig(path); cfg.MenuSounds {
		t.Error("menu_sounds false not loaded")
	}
}
