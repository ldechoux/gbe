package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ldechoux/gbe/internal/gb"
)

func TestRumbler(t *testing.T) {
	pads := newFakePads("Xbox Wireless Controller", "Generic USB Joystick")
	pads.pads[1].noVibration = true
	var r rumbler

	r.update(pads, true, 0.5) // averaged with the previous tick: 0.25
	if len(pads.vibrations) != 1 {
		t.Fatalf("vibrations %+v, want one, on the gamepad that vibrates", pads.vibrations)
	}
	if v := pads.vibrations[0]; v.id != 0 || v.strength != 0.25+0.75*0.25 || v.d != rumbleHold {
		t.Errorf("vibration %+v", v)
	}

	pads.vibrations = nil
	r.update(pads, true, 0) // still 0.25 on average
	r.update(pads, true, 0) // the motor stopped
	if n := len(pads.vibrations); n != 2 || pads.vibrations[1].strength != 0 {
		t.Errorf("vibrations %+v, want one more then a stop", pads.vibrations)
	}
	pads.vibrations = nil
	r.update(pads, true, 0)
	if len(pads.vibrations) != 0 {
		t.Errorf("vibrations %+v while already stopped", pads.vibrations)
	}

	r.update(pads, false, 1)
	if len(pads.vibrations) != 0 {
		t.Errorf("vibrations %+v with the option off", pads.vibrations)
	}
}

// rumbleGame runs a game whose cartridge has a motor, always on.
func rumbleGame(t *testing.T, pads *fakePads) *Game {
	t.Helper()
	rom := make([]byte, 0x8000)
	rom[0x147] = 0x1C // MBC5 + rumble
	copy(rom[0x100:], []byte{
		0x3E, 0x08, 0xEA, 0x00, 0x40, // LD A,0x08; LD (0x4000),A: motor on
		0x18, 0xFE, // JR -2
	})
	cart, err := gb.NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	g := newTestGame(t, pads)
	if g.gb, err = gb.New(cart, nil); err != nil {
		t.Fatal(err)
	}
	g.stream = &audioStream{}
	return g
}

func TestGameRumble(t *testing.T) {
	pads := newFakePads("Xbox Wireless Controller")
	g := rumbleGame(t, pads)
	g.advance(false, false)
	g.advance(false, false)
	if len(pads.vibrations) == 0 || pads.vibrations[len(pads.vibrations)-1].strength < 0.9 {
		t.Fatalf("vibrations %+v, want the gamepad shaking hard", pads.vibrations)
	}

	pads.vibrations = nil
	g.advance(false, true) // rewinding: no motor
	if len(pads.vibrations) != 1 || pads.vibrations[0].strength != 0 {
		t.Errorf("vibrations %+v when rewinding, want a stop", pads.vibrations)
	}

	pads.vibrations = nil
	g.cfg.Vibration = vibrationOff
	g.advance(false, false)
	if len(pads.vibrations) != 0 {
		t.Errorf("vibrations %+v with the option off", pads.vibrations)
	}

	// A game without a motor that plays an explosion shakes the gamepad
	// only in all games.
	for _, c := range []struct {
		mode  string
		shake bool
	}{{vibrationCartridge, false}, {vibrationAll, true}, {vibrationOff, false}} {
		pads.vibrations = nil
		g = withGameBoy(t, newTestGame(t, pads), explosion...)
		g.stream = &audioStream{}
		g.cfg.Vibration = c.mode
		g.advance(false, false)
		g.advance(false, false)
		shook := len(pads.vibrations) > 0 && pads.vibrations[len(pads.vibrations)-1].strength > 0.9
		if shook != c.shake || !c.shake && len(pads.vibrations) != 0 {
			t.Errorf("%s: vibrations %+v for a cartridge without a motor", c.mode, pads.vibrations)
		}
	}
}

// explosion plays loud low noise on both sides, for good.
var explosion = []byte{
	0x3E, 0x77, 0xE0, 0x24, // NR50: full master volume
	0x3E, 0xFF, 0xE0, 0x25, // NR51: every channel on both sides
	0x3E, 0xF0, 0xE0, 0x21, // NR42: volume 15, steady
	0x3E, 0x71, 0xE0, 0x22, // NR43: low noise
	0x3E, 0x80, 0xE0, 0x23, // NR44: trigger, no length
	0x18, 0xFE, // JR -2
}

func TestVibrationMenu(t *testing.T) {
	pads := newFakePads("Xbox Wireless Controller")
	g := newTestGame(t, pads)
	g.menu = menu{open: true, page: pageControls, tab: tabPad, cursor: padVibration()}
	l := g.tr()

	if v := g.menu.view(g); v.items[padVibration()] != "Vibration < Rumble carts >" || v.disabled[padVibration()] {
		t.Errorf("vibration entry %q, disabled %v", v.items[padVibration()], v.disabled[padVibration()])
	}
	g.actions = menuActions{ok: true}
	g.menu.update(g)
	if g.cfg.Vibration != vibrationAll {
		t.Errorf("Enter: vibration %q, want all games", g.cfg.Vibration)
	}
	g.actions = menuActions{right: true}
	g.menu.update(g)
	if g.cfg.Vibration != vibrationOff || g.menu.tab != tabPad {
		t.Errorf("Right on the vibration entry: vibration %q, tab %v; want it off, not the tab", g.cfg.Vibration, g.menu.tab)
	}
	g.actions = menuActions{left: true}
	g.menu.update(g)
	if g.cfg.Vibration != vibrationAll {
		t.Errorf("Left: vibration %q, want all games", g.cfg.Vibration)
	}
	if cfg, err := LoadConfig(g.cfgPath); err != nil || cfg.Vibration != vibrationAll {
		t.Errorf("vibration not saved: %q %v", cfg.Vibration, err)
	}

	g.menu.cursor = padTest()
	g.actions = menuActions{ok: true}
	g.menu.update(g)
	if len(pads.vibrations) != 1 || pads.vibrations[0] != (vibration{0, 1, 500 * time.Millisecond}) {
		t.Errorf("test vibrations %+v", pads.vibrations)
	}

	g.cfg.Vibration = vibrationOff
	g.menu.cursor = padDefaults()
	g.menu.update(g)
	if g.cfg.Vibration != vibrationCartridge {
		t.Errorf("Restore defaults: vibration %q, want the cartridges", g.cfg.Vibration)
	}

	// A gamepad that cannot vibrate: greyed out entries that do nothing.
	pads = newFakePads("Generic USB Joystick")
	pads.pads[0].noVibration = true
	g = newTestGame(t, pads)
	g.menu = menu{open: true, page: pageControls, tab: tabPad, cursor: padVibration()}
	v := g.menu.view(g)
	if !v.disabled[padVibration()] || !v.disabled[padTest()] || v.disabled[padDefaults()] {
		t.Errorf("disabled entries %v", v.disabled)
	}
	if !strings.HasSuffix(v.items[padVibration()], l.T("controls.unavailable")) || v.footer != l.T("controls.no_vibration") {
		t.Errorf("entry %q, footer %q", v.items[padVibration()], v.footer)
	}
	for _, c := range []int{padVibration(), padTest()} {
		g.menu.cursor = c
		g.actions = menuActions{ok: true}
		g.menu.update(g)
	}
	if g.cfg.Vibration != vibrationCartridge || len(pads.vibrations) != 0 {
		t.Errorf("greyed out entries acted: vibration %q, vibrations %+v", g.cfg.Vibration, pads.vibrations)
	}
}

func TestVibrationConfig(t *testing.T) {
	if v := DefaultConfig().Vibration; v != vibrationCartridge {
		t.Errorf("vibration %q by default, want the cartridges", v)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	// Absent, a boolean from before the modes, a mode, nonsense.
	for content, want := range map[string]string{
		`{"palette": "dmg"}`:      vibrationCartridge,
		`{"vibration": true}`:     vibrationCartridge,
		`{"vibration": false}`:    vibrationOff,
		`{"vibration": "all"}`:    vibrationAll,
		`{"vibration": "strong"}`: vibrationCartridge,
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if cfg, err := LoadConfig(path); err != nil || cfg.Vibration != want {
			t.Errorf("%s: vibration %q (%v), want %q", content, cfg.Vibration, err, want)
		}
	}
	cfg := DefaultConfig()
	cfg.Vibration = vibrationOff
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	if cfg, err := LoadConfig(path); err != nil || cfg.Vibration != vibrationOff {
		t.Errorf("vibration %q after saving it off (%v)", cfg.Vibration, err)
	}
}
