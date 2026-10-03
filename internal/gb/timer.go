package gb

// Timer implements DIV/TIMA/TMA/TAC on top of the 16-bit system counter.
// TIMA increments on the falling edge of a counter bit selected by TAC.
type Timer struct {
	bus *Bus

	counter  uint16 // DIV is the upper byte
	tima     byte
	tma      byte
	tac      byte
	overflow bool // TIMA overflowed during the previous M-cycle
}

var timerBits = [4]uint16{1 << 9, 1 << 3, 1 << 5, 1 << 7}

func (t *Timer) signal() bool {
	return t.tac&4 != 0 && t.counter&timerBits[t.tac&3] != 0
}

func (t *Timer) incTIMA() {
	t.tima++
	if t.tima == 0 {
		t.overflow = true
	}
}

// tick advances the timer by one M-cycle.
func (t *Timer) tick() {
	if t.overflow {
		// TIMA reads 0 for one M-cycle before being reloaded.
		t.overflow = false
		t.tima = t.tma
		t.bus.requestInterrupt(IntTimer)
	}
	// The bits of the counter that fall: TIMA and the frame sequencer of
	// the APU count falling edges. Every other M-cycle, none does.
	old := t.counter
	t.counter += 4
	fell := old &^ t.counter
	if fell == 0 {
		return
	}
	if t.tac&4 != 0 && fell&timerBits[t.tac&3] != 0 {
		t.incTIMA()
	}
	if fell&t.divEventBit() != 0 {
		t.bus.apu.divEvent()
	}
}

func (t *Timer) resetDiv() {
	old := t.signal()
	div := t.counter&t.divEventBit() != 0
	t.counter = 0
	if old {
		t.incTIMA()
	}
	if div { // a falling edge for the APU as well
		t.bus.apu.divEvent()
	}
}

// divEventBit is the bit of the counter whose falling edges clock the frame
// sequencer of the APU: bit 4 of DIV, bit 5 in double speed mode (where the
// counter runs twice as fast).
func (t *Timer) divEventBit() uint16 {
	if t.bus.doubleSpeed {
		return 1 << 13
	}
	return 1 << 12
}

func (t *Timer) read(addr uint16) byte {
	switch addr {
	case 0xFF04:
		return byte(t.counter >> 8)
	case 0xFF05:
		return t.tima
	case 0xFF06:
		return t.tma
	}
	return t.tac | 0xF8
}

func (t *Timer) write(addr uint16, v byte) {
	switch addr {
	case 0xFF04:
		t.resetDiv()
	case 0xFF05:
		t.tima = v
		t.overflow = false
	case 0xFF06:
		t.tma = v
	case 0xFF07:
		old := t.signal()
		t.tac = v & 7
		if old && !t.signal() {
			t.incTIMA()
		}
	}
}
