package ui

import (
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/i18n"
)

// Gamepads are read through Ebitengine's standard layout (mapped with the
// SDL controller database), where buttons are named by position: the bottom
// face button is RightBottom whatever its printed label. Gamepads without a
// standard mapping are ignored.

// padButton is a standard layout button, stored by name in the config.
type padButton ebiten.StandardGamepadButton

// padNone marks an unknown button name found in a config file.
const padNone padButton = -1

var padButtonNames = map[padButton]string{
	padButton(ebiten.StandardGamepadButtonRightBottom):      "RightBottom",
	padButton(ebiten.StandardGamepadButtonRightRight):       "RightRight",
	padButton(ebiten.StandardGamepadButtonRightLeft):        "RightLeft",
	padButton(ebiten.StandardGamepadButtonRightTop):         "RightTop",
	padButton(ebiten.StandardGamepadButtonFrontTopLeft):     "FrontTopLeft",
	padButton(ebiten.StandardGamepadButtonFrontTopRight):    "FrontTopRight",
	padButton(ebiten.StandardGamepadButtonFrontBottomLeft):  "FrontBottomLeft",
	padButton(ebiten.StandardGamepadButtonFrontBottomRight): "FrontBottomRight",
	padButton(ebiten.StandardGamepadButtonCenterLeft):       "CenterLeft",
	padButton(ebiten.StandardGamepadButtonCenterRight):      "CenterRight",
	padButton(ebiten.StandardGamepadButtonLeftStick):        "LeftStick",
	padButton(ebiten.StandardGamepadButtonRightStick):       "RightStick",
	padButton(ebiten.StandardGamepadButtonLeftTop):          "LeftTop",
	padButton(ebiten.StandardGamepadButtonLeftBottom):       "LeftBottom",
	padButton(ebiten.StandardGamepadButtonLeftLeft):         "LeftLeft",
	padButton(ebiten.StandardGamepadButtonLeftRight):        "LeftRight",
	padButton(ebiten.StandardGamepadButtonCenterCenter):     "CenterCenter",
}

func (b padButton) MarshalText() ([]byte, error) { return []byte(padButtonNames[b]), nil }

// UnmarshalText never fails: an unknown name becomes padNone and is
// replaced by the default mapping when the config is loaded.
func (b *padButton) UnmarshalText(text []byte) error {
	*b = padNone
	for btn, name := range padButtonNames {
		if name == string(text) {
			*b = btn
		}
	}
	return nil
}

func (b padButton) std() ebiten.StandardGamepadButton { return ebiten.StandardGamepadButton(b) }

// defaultPad mirrors the Game Boy: B on the bottom face button, A on the
// right one (the Nintendo layout).
func defaultPad() map[string]padButton {
	return map[string]padButton{
		gb.ButtonUp.String():     padButton(ebiten.StandardGamepadButtonLeftTop),
		gb.ButtonDown.String():   padButton(ebiten.StandardGamepadButtonLeftBottom),
		gb.ButtonLeft.String():   padButton(ebiten.StandardGamepadButtonLeftLeft),
		gb.ButtonRight.String():  padButton(ebiten.StandardGamepadButtonLeftRight),
		gb.ButtonA.String():      padButton(ebiten.StandardGamepadButtonRightRight),
		gb.ButtonB.String():      padButton(ebiten.StandardGamepadButtonRightBottom),
		gb.ButtonStart.String():  padButton(ebiten.StandardGamepadButtonCenterRight),
		gb.ButtonSelect.String(): padButton(ebiten.StandardGamepadButtonCenterLeft),
	}
}

// padFamily selects the button labels matching what is printed on the pad.
type padFamily int

const (
	padXbox padFamily = iota
	padPlayStation
	padNintendo
)

func (f padFamily) String() string {
	return [...]string{"Xbox", "PlayStation", "Nintendo"}[f]
}

func detectFamily(name string) padFamily {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "xbox"): // "Xbox Wireless Controller" would match the DualShock 4 below
		return padXbox
	case containsAny(n, "dualsense", "dualshock", "playstation", "ps3", "ps4", "ps5", "wireless controller"):
		return padPlayStation
	case containsAny(n, "nintendo", "pro controller", "joy-con", "switch"):
		return padNintendo
	}
	return padXbox
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// padLabels: face buttons in the order bottom, right, left, top, then
// shoulders and center buttons, per family. Labels starting with "pad." are
// message keys, translated; the others are printed as is.
var padLabels = map[padFamily]map[padButton]string{
	padXbox: {
		padButton(ebiten.StandardGamepadButtonRightBottom):      "A",
		padButton(ebiten.StandardGamepadButtonRightRight):       "B",
		padButton(ebiten.StandardGamepadButtonRightLeft):        "X",
		padButton(ebiten.StandardGamepadButtonRightTop):         "Y",
		padButton(ebiten.StandardGamepadButtonFrontTopLeft):     "LB",
		padButton(ebiten.StandardGamepadButtonFrontTopRight):    "RB",
		padButton(ebiten.StandardGamepadButtonFrontBottomLeft):  "LT",
		padButton(ebiten.StandardGamepadButtonFrontBottomRight): "RT",
		padButton(ebiten.StandardGamepadButtonCenterLeft):       "pad.view",
		padButton(ebiten.StandardGamepadButtonCenterRight):      "Menu",
		padButton(ebiten.StandardGamepadButtonCenterCenter):     "Xbox",
	},
	padPlayStation: {
		padButton(ebiten.StandardGamepadButtonRightBottom):      "pad.cross",
		padButton(ebiten.StandardGamepadButtonRightRight):       "pad.circle",
		padButton(ebiten.StandardGamepadButtonRightLeft):        "pad.square",
		padButton(ebiten.StandardGamepadButtonRightTop):         "pad.triangle",
		padButton(ebiten.StandardGamepadButtonFrontTopLeft):     "L1",
		padButton(ebiten.StandardGamepadButtonFrontTopRight):    "R1",
		padButton(ebiten.StandardGamepadButtonFrontBottomLeft):  "L2",
		padButton(ebiten.StandardGamepadButtonFrontBottomRight): "R2",
		padButton(ebiten.StandardGamepadButtonCenterLeft):       "Share",
		padButton(ebiten.StandardGamepadButtonCenterRight):      "Options",
		padButton(ebiten.StandardGamepadButtonCenterCenter):     "PS",
	},
	padNintendo: {
		padButton(ebiten.StandardGamepadButtonRightBottom):      "B",
		padButton(ebiten.StandardGamepadButtonRightRight):       "A",
		padButton(ebiten.StandardGamepadButtonRightLeft):        "Y",
		padButton(ebiten.StandardGamepadButtonRightTop):         "X",
		padButton(ebiten.StandardGamepadButtonFrontTopLeft):     "L",
		padButton(ebiten.StandardGamepadButtonFrontTopRight):    "R",
		padButton(ebiten.StandardGamepadButtonFrontBottomLeft):  "ZL",
		padButton(ebiten.StandardGamepadButtonFrontBottomRight): "ZR",
		padButton(ebiten.StandardGamepadButtonCenterLeft):       "-",
		padButton(ebiten.StandardGamepadButtonCenterRight):      "+",
		padButton(ebiten.StandardGamepadButtonCenterCenter):     "Home",
	},
}

// Buttons labeled the same on every family.
var padCommonLabels = map[padButton]string{
	padButton(ebiten.StandardGamepadButtonLeftTop):    "pad.dpad_up",
	padButton(ebiten.StandardGamepadButtonLeftBottom): "pad.dpad_down",
	padButton(ebiten.StandardGamepadButtonLeftLeft):   "pad.dpad_left",
	padButton(ebiten.StandardGamepadButtonLeftRight):  "pad.dpad_right",
	padButton(ebiten.StandardGamepadButtonLeftStick):  "pad.left_stick",
	padButton(ebiten.StandardGamepadButtonRightStick): "pad.right_stick",
}

// padLabel names the gamepad button b of family f in language l.
func padLabel(l *i18n.Locale, f padFamily, b padButton) string {
	s, ok := padLabels[f][b]
	if !ok {
		if s, ok = padCommonLabels[b]; !ok {
			return "?"
		}
	}
	if strings.HasPrefix(s, "pad.") {
		return l.T(s)
	}
	return s
}

// padReader abstracts gamepad polling so the logic can be tested without
// hardware.
type padReader interface {
	// ids lists the connected gamepads that have a standard mapping.
	ids() []ebiten.GamepadID
	name(id ebiten.GamepadID) string
	// duration is how many ticks the button has been held (0: released).
	duration(id ebiten.GamepadID, b padButton) int
	axis(id ebiten.GamepadID, a ebiten.StandardGamepadAxis) float64
}

// ebitenPads reads the real gamepads.
type ebitenPads struct{ buf []ebiten.GamepadID }

func (p *ebitenPads) ids() []ebiten.GamepadID {
	p.buf = ebiten.AppendGamepadIDs(p.buf[:0])
	out := p.buf[:0]
	for _, id := range p.buf {
		if ebiten.IsStandardGamepadLayoutAvailable(id) {
			out = append(out, id)
		}
	}
	return out
}

func (p *ebitenPads) name(id ebiten.GamepadID) string { return ebiten.GamepadName(id) }

func (p *ebitenPads) duration(id ebiten.GamepadID, b padButton) int {
	return inpututil.StandardGamepadButtonPressDuration(id, b.std())
}

func (p *ebitenPads) axis(id ebiten.GamepadID, a ebiten.StandardGamepadAxis) float64 {
	return ebiten.StandardGamepadAxisValue(id, a)
}

// stickThreshold is how far the left stick must be pushed to act as the
// D-pad.
const stickThreshold = 0.5

// stickDirs returns the D-pad directions the left stick points to.
func stickDirs(r padReader, id ebiten.GamepadID) (up, down, left, right bool) {
	x := r.axis(id, ebiten.StandardGamepadAxisLeftStickHorizontal)
	y := r.axis(id, ebiten.StandardGamepadAxisLeftStickVertical)
	return y < -stickThreshold, y > stickThreshold, x < -stickThreshold, x > stickThreshold
}

// padHeld reports whether button b is held on any gamepad.
func padHeld(r padReader, b padButton) bool {
	for _, id := range r.ids() {
		if r.duration(id, b) > 0 {
			return true
		}
	}
	return false
}

// padJustPressed returns the first button pressed this tick on any gamepad.
func padJustPressed(r padReader) (padButton, bool) {
	for _, id := range r.ids() {
		for b := range padButtonNames {
			if r.duration(id, b) == 1 {
				return b, true
			}
		}
	}
	return padNone, false
}

// padGameButtons merges every gamepad into the Game Boy button states,
// indexed by gb.Button. Buttons in ignored stay released (see Game).
func padGameButtons(r padReader, mapping map[string]padButton, ignored map[padButton]bool) (out [8]bool) {
	for _, id := range r.ids() {
		for _, b := range gb.Buttons {
			pb := mapping[b.String()]
			if !ignored[pb] && r.duration(id, pb) > 0 {
				out[b] = true
			}
		}
		up, down, left, right := stickDirs(r, id)
		out[gb.ButtonUp] = out[gb.ButtonUp] || up
		out[gb.ButtonDown] = out[gb.ButtonDown] || down
		out[gb.ButtonLeft] = out[gb.ButtonLeft] || left
		out[gb.ButtonRight] = out[gb.ButtonRight] || right
	}
	return out
}

// padName describes the first gamepad, for the controls page.
func padName(r padReader) (string, padFamily, bool) {
	ids := r.ids()
	if len(ids) == 0 {
		return "", padXbox, false
	}
	n := r.name(ids[0])
	return n, detectFamily(n), true
}
