package gb

// Button identifies one of the eight Game Boy inputs.
type Button int

const (
	ButtonRight Button = iota
	ButtonLeft
	ButtonUp
	ButtonDown
	ButtonA
	ButtonB
	ButtonSelect
	ButtonStart
)

// Buttons lists every button, in a stable order.
var Buttons = []Button{ButtonUp, ButtonDown, ButtonLeft, ButtonRight, ButtonA, ButtonB, ButtonStart, ButtonSelect}

var buttonNames = map[Button]string{
	ButtonRight: "Right", ButtonLeft: "Left", ButtonUp: "Up", ButtonDown: "Down",
	ButtonA: "A", ButtonB: "B", ButtonSelect: "Select", ButtonStart: "Start",
}

func (b Button) String() string { return buttonNames[b] }

// Joypad implements the P1 register (0xFF00).
type Joypad struct {
	bus     *Bus
	sel     byte // bits 4-5 as last written
	pressed [8]bool
}

func (j *Joypad) lowNibble() byte {
	v := byte(0x0F)
	if j.sel&0x10 == 0 { // direction keys
		for i, b := range []Button{ButtonRight, ButtonLeft, ButtonUp, ButtonDown} {
			if j.pressed[b] {
				v &^= 1 << i
			}
		}
	}
	if j.sel&0x20 == 0 { // action keys
		for i, b := range []Button{ButtonA, ButtonB, ButtonSelect, ButtonStart} {
			if j.pressed[b] {
				v &^= 1 << i
			}
		}
	}
	return v
}

func (j *Joypad) read() byte { return 0xC0 | j.sel | j.lowNibble() }

func (j *Joypad) write(v byte) { j.sel = v & 0x30 }

func (j *Joypad) set(b Button, down bool) {
	before := j.lowNibble()
	j.pressed[b] = down
	if before&^j.lowNibble() != 0 { // a line went from high to low
		j.bus.requestInterrupt(IntJoypad)
	}
}
