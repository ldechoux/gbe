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
	// signal(), inlined: the bit of the counter TIMA follows, 0 when off.
	var mask uint16
	if t.tac&4 != 0 {
		mask = timerBits[t.tac&3]
	}
	old := t.counter & mask
	t.counter += 4
	if old != 0 && t.counter&mask == 0 {
		t.incTIMA()
	}
}

func (t *Timer) resetDiv() {
	old := t.signal()
	t.counter = 0
	if old {
		t.incTIMA()
	}
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
