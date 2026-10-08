package gb

import "math"

// While the CPU is halted, most M-cycles only move the clocks forward: the
// system counter of the timer, the dot of the PPU, the timers and the sample
// clock of the APU. The bus runs those all at once (skipQuiet), up to the
// next M-cycle in which a component does more, which runs as usual: only
// such an M-cycle may request the interrupt that ends the halt. The state is
// the same as after running them one by one.

// quietTicks is how many M-cycles from now the bus can run before the first
// one in which a component does more than count, within the frame being run.
func (b *Bus) quietTicks() int {
	if b.serial.remaining > 0 {
		return 0
	}
	dots := b.dots()
	// The APU first: while music plays, its next event is often the next
	// M-cycle, and the others need not be looked at.
	n := b.apu.ticksToEvent(dots)
	if n <= 1 {
		return 0
	}
	n = min(n, b.timer.ticksToEvent(), b.ppu.ticksToEvent(dots))
	// RunFrame stops after the M-cycle that reaches frameEnd, as without
	// skipping: the buttons may change between frames.
	left := int((b.frameEnd - b.cycles + uint64(dots) - 1) / uint64(dots))
	return min(n, left) - 1
}

// skipQuiet runs n M-cycles in which no component does more than count (see
// quietTicks).
func (b *Bus) skipQuiet(n int) {
	dots := b.dots()
	b.timer.counter += uint16(4 * n)
	if b.ppu.lcdc&0x80 != 0 {
		b.ppu.dot += dots * n
	}
	b.apu.idle(dots * n)
	b.cycles += uint64(dots * n)
}

// dots is the number of T-cycles at the normal speed of an M-cycle.
func (b *Bus) dots() int {
	if b.doubleSpeed {
		return 2
	}
	return 4
}

// ticksToEvent is in how many M-cycles the timer does more than count: TIMA
// is reloaded after an overflow, it is incremented, or the frame sequencer
// of the APU is clocked.
func (t *Timer) ticksToEvent() int {
	if t.overflow {
		return 1
	}
	n := t.ticksToFall(t.divEventBit())
	if t.tac&4 != 0 {
		n = min(n, t.ticksToFall(timerBits[t.tac&3]))
	}
	return n
}

// ticksToFall is in how many M-cycles bit of the system counter falls: when
// the counter, 4 more each M-cycle, reaches the next multiple of twice it.
func (t *Timer) ticksToFall(bit uint16) int {
	m := uint32(bit) << 1
	c := uint32(t.counter)
	next := (c/m + 1) * m
	return int((next - c + 3) / 4)
}

// ticksToEvent is in how many M-cycles of the given number of dots the PPU
// changes mode or line: the M-cycle in which its dot reaches the end of the
// current mode.
func (p *PPU) ticksToEvent(dots int) int {
	if p.lcdc&0x80 == 0 {
		return math.MaxInt
	}
	end := 456
	switch p.mode {
	case 2:
		end = 80
	case 3:
		end = 80 + 172
	}
	return max(1, (end-p.dot+dots-1)/dots)
}

// ticksToEvent is in how many M-cycles of the given number of T-cycles the
// APU does more than count: the output changes, the timer of an enabled
// channel expires, or an output sample ends.
func (a *APU) ticksToEvent(cycles int) int {
	if a.dirty != 0 {
		return 1
	}
	n := math.MaxInt
	if a.on {
		// Before the sample, which takes floating-point math: while music
		// plays, a timer often expires in the next M-cycle.
		if n = max(1, (a.untilClock-a.pending+cycles-1)/cycles); n == 1 {
			return 1
		}
	}
	return min(n, a.ticksToSample(cycles))
}

// ticksToSample is in how many M-cycles of the given number of T-cycles the
// output sample ends, as mix sees it.
func (a *APU) ticksToSample(cycles int) int {
	n := 1
	if left := a.samplePeriod - a.sampleClock; left > float64(cycles) {
		n = int(math.Ceil(left / float64(cycles)))
		// The division may round: settle on the first M-cycle that ends
		// the sample.
		for n > 1 && a.sampleClock+float64((n-1)*cycles) >= a.samplePeriod {
			n--
		}
		for a.sampleClock+float64(n*cycles) < a.samplePeriod {
			n++
		}
	}
	return n
}

// idle moves the clocks of the APU forward by T-cycles in which no timer of
// an enabled channel expires, no output sample ends and the output does not
// change. sampleClock stays a multiple of the precision of samplePeriod,
// below twice it, so adding them at once gives the same as one by one.
func (a *APU) idle(cycles int) {
	if a.on {
		a.pending += cycles
	}
	a.sampleClock += float64(cycles)
}
