package ui

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/i18n"
)

// fakePads is a scriptable padReader.
type fakePads struct {
	pads map[ebiten.GamepadID]*fakePad
}

type fakePad struct {
	name string
	held map[padButton]int
	axes map[ebiten.StandardGamepadAxis]float64
}

func newFakePads(names ...string) *fakePads {
	f := &fakePads{pads: map[ebiten.GamepadID]*fakePad{}}
	for i, n := range names {
		f.pads[ebiten.GamepadID(i)] = &fakePad{name: n, held: map[padButton]int{}, axes: map[ebiten.StandardGamepadAxis]float64{}}
	}
	return f
}

func (f *fakePads) ids() []ebiten.GamepadID {
	var ids []ebiten.GamepadID
	for id := range ebiten.GamepadID(len(f.pads)) {
		if _, ok := f.pads[id]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}
func (f *fakePads) name(id ebiten.GamepadID) string { return f.pads[id].name }
func (f *fakePads) duration(id ebiten.GamepadID, b padButton) int {
	return f.pads[id].held[b]
}
func (f *fakePads) axis(id ebiten.GamepadID, a ebiten.StandardGamepadAxis) float64 {
	return f.pads[id].axes[a]
}

// tick advances every held button by one tick, like Ebitengine does.
func (f *fakePads) tick() {
	for _, p := range f.pads {
		for b, d := range p.held {
			if d > 0 {
				p.held[b] = d + 1
			}
		}
	}
}

func (f *fakePads) press(id ebiten.GamepadID, b padButton)   { f.pads[id].held[b] = 1 }
func (f *fakePads) release(id ebiten.GamepadID, b padButton) { f.pads[id].held[b] = 0 }

func std(b ebiten.StandardGamepadButton) padButton { return padButton(b) }

func TestGamepadConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := DefaultConfig()
	cfg.BindPad(gb.ButtonA.String(), std(ebiten.StandardGamepadButtonRightBottom))
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range gb.Buttons {
		if got.PadButton(b) != cfg.PadButton(b) {
			t.Errorf("%s: %v, want %v", b, got.PadButton(b), cfg.PadButton(b))
		}
	}

	// Configs from older versions, or with an unknown button, get defaults.
	os.WriteFile(path, []byte(`{"gamepad":{"A":"NoSuchButton"}}`), 0o644)
	got, err = LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.PadButton(gb.ButtonA) != defaultPad()["A"] || got.PadButton(gb.ButtonB) != defaultPad()["B"] {
		t.Fatalf("defaults not applied: %v", got.Gamepad)
	}
}

func TestBindPadSwapsConflicts(t *testing.T) {
	cfg := DefaultConfig()
	a, b := cfg.PadButton(gb.ButtonA), cfg.PadButton(gb.ButtonB)
	cfg.BindPad(gb.ButtonA.String(), b)
	if cfg.PadButton(gb.ButtonA) != b || cfg.PadButton(gb.ButtonB) != a {
		t.Fatalf("A=%v B=%v", cfg.PadButton(gb.ButtonA), cfg.PadButton(gb.ButtonB))
	}
}

func TestPadGameButtons(t *testing.T) {
	pads := newFakePads("Xbox Wireless Controller", "Pro Controller")
	cfg := DefaultConfig()
	pads.press(0, cfg.PadButton(gb.ButtonA))
	pads.press(1, cfg.PadButton(gb.ButtonStart)) // second pad counts too
	pads.pads[0].axes[ebiten.StandardGamepadAxisLeftStickHorizontal] = -0.8
	pads.pads[1].axes[ebiten.StandardGamepadAxisLeftStickVertical] = 0.3 // below the threshold

	got := padGameButtons(pads, cfg.Gamepad, map[padButton]bool{})
	for _, b := range gb.Buttons {
		want := b == gb.ButtonA || b == gb.ButtonStart || b == gb.ButtonLeft
		if got[b] != want {
			t.Errorf("%s = %v, want %v", b, got[b], want)
		}
	}
	ignored := map[padButton]bool{cfg.PadButton(gb.ButtonA): true}
	if padGameButtons(pads, cfg.Gamepad, ignored)[gb.ButtonA] {
		t.Error("ignored buttons must not reach the game")
	}
}

func TestPadFamilyLabels(t *testing.T) {
	for name, want := range map[string]padFamily{
		"Xbox Wireless Controller":       padXbox,
		"Wireless Controller":            padPlayStation, // DualShock 4
		"DualSense Wireless Controller":  padPlayStation,
		"Nintendo Switch Pro Controller": padNintendo,
		"Generic USB Joystick":           padXbox,
	} {
		if got := detectFamily(name); got != want {
			t.Errorf("detectFamily(%q) = %s, want %s", name, got, want)
		}
	}
	// The default A (right face button) as printed on each family.
	a := defaultPad()["A"]
	en, fr := i18n.Get("en"), i18n.Get("fr")
	for f, want := range map[padFamily]string{padXbox: "B", padPlayStation: "Rond", padNintendo: "A"} {
		if got := padLabel(fr, f, a); got != want {
			t.Errorf("%s: %q, want %q", f, got, want)
		}
	}
	if got := padLabel(en, padPlayStation, a); got != "Circle" {
		t.Errorf("English PlayStation label %q", got)
	}
	if got := padLabel(fr, padXbox, std(ebiten.StandardGamepadButtonLeftTop)); got != "Croix haut" {
		t.Errorf("D-pad label %q", got)
	}
	// Every translated label must exist.
	for _, labels := range append(slices.Collect(maps.Values(padLabels)), padCommonLabels) {
		for _, s := range labels {
			if strings.HasPrefix(s, "pad.") && !en.Has(s) {
				t.Errorf("no message %q", s)
			}
		}
	}
}

func TestPadMenuActions(t *testing.T) {
	pads := newFakePads("Xbox Wireless Controller")
	cfg := DefaultConfig()
	var st stickNav

	pads.press(0, cfg.PadButton(gb.ButtonDown))
	pads.press(0, cfg.PadButton(gb.ButtonA))
	a := padActions(pads, cfg.Gamepad, &st)
	if !a.down || !a.ok || a.back || a.toggle {
		t.Fatalf("first tick: %+v", a)
	}
	pads.tick()
	if a = padActions(pads, cfg.Gamepad, &st); a.down || a.ok {
		t.Fatalf("held buttons must not fire again right away: %+v", a)
	}
	repeats := 0
	for range 40 {
		pads.tick()
		if padActions(pads, cfg.Gamepad, &st).down {
			repeats++
		}
	}
	if repeats == 0 {
		t.Fatal("a held direction auto-repeats")
	}

	// Start+Select toggles the menu once.
	pads = newFakePads("x")
	pads.press(0, cfg.PadButton(gb.ButtonStart))
	if padActions(pads, cfg.Gamepad, &st).toggle {
		t.Fatal("Start alone must not toggle")
	}
	pads.tick()
	pads.press(0, cfg.PadButton(gb.ButtonSelect))
	if !padActions(pads, cfg.Gamepad, &st).toggle {
		t.Fatal("Start+Select must toggle")
	}
	pads.tick()
	if padActions(pads, cfg.Gamepad, &st).toggle {
		t.Fatal("holding Start+Select must toggle only once")
	}

	// The stick navigates too.
	pads = newFakePads("x")
	st = stickNav{}
	pads.pads[0].axes[ebiten.StandardGamepadAxisLeftStickVertical] = -1
	if !padActions(pads, cfg.Gamepad, &st).up {
		t.Fatal("stick up must move up")
	}
	if padActions(pads, cfg.Gamepad, &st).up {
		t.Fatal("the stick repeats like a held button, not every tick")
	}
}

// newTestGame builds a Game without a window or audio.
func newTestGame(t *testing.T, pads padReader) *Game {
	t.Helper()
	return &Game{cfg: DefaultConfig(), cfgPath: filepath.Join(t.TempDir(), "c.json"), pads: pads,
		ignoredKeys: map[ebiten.Key]bool{}, ignoredPad: map[padButton]bool{}}
}

func TestControlsTabs(t *testing.T) {
	pads := newFakePads()
	g := newTestGame(t, pads)
	g.menu = menu{open: true, page: pageControls}

	g.actions = menuActions{right: true}
	g.menu.update(g)
	if g.menu.tab != tabKeyboard || g.menu.notice != "" {
		t.Fatalf("without a gamepad, Right must do nothing: %+v", g.menu)
	}
	if v := g.menu.view(g); !v.tabs[1].disabled {
		t.Fatal("the Gamepad tab must be shown disabled")
	}

	pads.pads[0] = &fakePad{name: "Pro Controller", held: map[padButton]int{}, axes: map[ebiten.StandardGamepadAxis]float64{}}
	g.menu.update(g)
	if g.menu.tab != tabPad {
		t.Fatal("the Gamepad tab must open once a gamepad is connected")
	}

	// Rebind A from the gamepad tab.
	g.actions = menuActions{ok: true}
	g.menu.update(g) // cursor 0 = Up: start capturing
	if !g.menu.capturing {
		t.Fatal("OK on a button must start capturing")
	}
	pads.press(0, std(ebiten.StandardGamepadButtonRightTop))
	g.menu.update(g)
	if g.menu.capturing || g.cfg.PadButton(gb.ButtonUp) != std(ebiten.StandardGamepadButtonRightTop) {
		t.Fatalf("capture failed: capturing=%v up=%v", g.menu.capturing, g.cfg.PadButton(gb.ButtonUp))
	}

	// Unplugging falls back to the keyboard tab.
	delete(pads.pads, 0)
	g.actions = menuActions{}
	g.menu.update(g)
	if g.menu.tab != tabKeyboard || g.menu.notice != "Gamepad disconnected" {
		t.Fatalf("after unplugging: %+v", g.menu)
	}
}
