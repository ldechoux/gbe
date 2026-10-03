package gb

import "testing"

// triggerCh1 sets up channel 1 at full volume with the given sweep (NR10)
// and frequency, then triggers it.
func triggerCh1(a *APU, nr10 byte, freq int) {
	a.write(0xFF10, nr10)
	a.write(0xFF12, 0xF0)
	a.write(0xFF13, byte(freq))
	a.write(0xFF14, 0x80|byte(freq>>8))
}

func TestAPUSweep(t *testing.T) {
	g := newTestGB(t)
	a := g.APU

	// Period 1, increase, shift 1: 0x400 -> 0x600, then 0x900 overflows.
	triggerCh1(a, 0x11, 0x400)
	if !a.ch[0].enabled {
		t.Fatal("channel 1 not enabled by the trigger")
	}
	a.sweepStep()
	if a.ch[0].freq != 0x600 {
		t.Errorf("frequency %#x after one sweep, want 0x600", a.ch[0].freq)
	}
	if a.regs[3] != 0x00 || a.regs[4]&7 != 6 {
		t.Errorf("NR13/NR14 not updated: %02X %02X", a.regs[3], a.regs[4])
	}
	if a.ch[0].enabled || a.read(0xFF26)&1 != 0 {
		t.Error("the overflow check after the sweep did not disable channel 1")
	}

	// The overflow check also runs on trigger.
	triggerCh1(a, 0x11, 0x7FF)
	if a.ch[0].enabled {
		t.Error("trigger with an overflowing sweep left channel 1 enabled")
	}

	// Decrease: 0x400 -> 0x200.
	triggerCh1(a, 0x19, 0x400)
	a.sweepStep()
	if a.ch[0].freq != 0x200 || !a.ch[0].enabled {
		t.Errorf("negate sweep: freq %#x enabled %v, want 0x200 true", a.ch[0].freq, a.ch[0].enabled)
	}
	// Leaving negate mode after a negate calculation disables the channel.
	a.write(0xFF10, 0x11)
	if a.ch[0].enabled {
		t.Error("clearing negate after a negate calculation left channel 1 enabled")
	}

	// A sweep period of 0 never changes the frequency.
	triggerCh1(a, 0x01, 0x400)
	for range 16 {
		a.sweepStep()
	}
	if a.ch[0].freq != 0x400 || !a.ch[0].enabled {
		t.Errorf("period 0: freq %#x enabled %v, want 0x400 true", a.ch[0].freq, a.ch[0].enabled)
	}

	// The sweep timer counts down the period before acting.
	triggerCh1(a, 0x31, 0x100) // period 3
	a.sweepStep()
	a.sweepStep()
	if a.ch[0].freq != 0x100 {
		t.Errorf("sweep acted early: freq %#x", a.ch[0].freq)
	}
	a.sweepStep()
	if a.ch[0].freq != 0x180 {
		t.Errorf("freq %#x after 3 steps, want 0x180", a.ch[0].freq)
	}
}

func TestAPULength(t *testing.T) {
	g := newTestGB(t)
	a := g.APU
	a.write(0xFF17, 0xF0) // channel 2 DAC on

	a.write(0xFF16, 0x3E) // length 2
	a.write(0xFF19, 0xC0) // trigger, length enabled
	a.lengthSteps()
	if !a.ch[1].enabled {
		t.Fatal("channel 2 stopped after 1 of 2 length steps")
	}
	a.lengthSteps()
	if a.ch[1].enabled || a.read(0xFF26)&2 != 0 {
		t.Error("channel 2 still enabled when its length expired")
	}

	a.write(0xFF19, 0x80) // trigger, length disabled: length reloads to 64
	if a.ch[1].length != 64 {
		t.Errorf("length %d after trigger, want 64", a.ch[1].length)
	}
	for range 100 {
		a.lengthSteps()
	}
	if !a.ch[1].enabled {
		t.Error("channel 2 stopped although its length counter is disabled")
	}

	a.write(0xFF1A, 0x80)
	a.write(0xFF1B, 0xFF) // wave length 1
	a.write(0xFF1E, 0xC0)
	a.lengthSteps()
	if a.ch[2].enabled {
		t.Error("channel 3 still enabled when its length expired")
	}

	// The frame sequencer clocks length on even steps only.
	a.write(0xFF16, 0x3F) // length 1
	a.write(0xFF19, 0xC0)
	a.seqStep = 1
	a.sequencerStep()
	if !a.ch[1].enabled {
		t.Error("sequencer step 1 clocked the length counter")
	}
	a.sequencerStep()
	if a.ch[1].enabled {
		t.Error("sequencer step 2 did not clock the length counter")
	}
}

func TestEnvelope(t *testing.T) {
	e := envelope{}
	e.load(0x52) // volume 5, decrease, period 2
	e.trigger()
	e.step()
	if e.volume != 5 {
		t.Errorf("volume %d after 1 of 2 steps, want 5", e.volume)
	}
	e.step()
	if e.volume != 4 {
		t.Errorf("volume %d after 2 steps, want 4", e.volume)
	}

	e.load(0xE9) // volume 14, increase, period 1
	e.trigger()
	for range 5 {
		e.step()
	}
	if e.volume != 15 {
		t.Errorf("volume %d, want 15 (clamped)", e.volume)
	}

	e.load(0x08) // period 0: frozen
	e.trigger()
	e.step()
	if e.volume != 0 || e.timer != 8 {
		t.Errorf("period 0: volume %d timer %d, want 0 8", e.volume, e.timer)
	}
}

func TestAPUOutput(t *testing.T) {
	g := newTestGB(t)
	a := g.APU

	a.write(0xFF11, 0x80) // duty 50%: 1,0,0,0,0,1,1,1
	triggerCh1(a, 0x00, 0)
	a.ch[0].dutyPos = 0
	if got := a.output(0); got != 15 {
		t.Errorf("square high: %d, want 15", got)
	}
	a.ch[0].dutyPos = 1
	if got := a.output(0); got != 0 {
		t.Errorf("square low: %d, want 0", got)
	}

	a.write(0xFF30, 0xA5)
	a.write(0xFF1A, 0x80)
	a.write(0xFF1E, 0x80)
	for _, c := range []struct {
		nr32 byte
		pos  byte
		want byte
	}{
		{0x20, 0, 0xA}, {0x20, 1, 0x5}, {0x40, 0, 0x5}, {0x60, 0, 0x2}, {0x00, 0, 0},
	} {
		a.write(0xFF1C, c.nr32)
		a.ch[2].wavePos = c.pos
		if got := a.output(2); got != c.want {
			t.Errorf("wave NR32=%02X pos %d: %d, want %d", c.nr32, c.pos, got, c.want)
		}
	}

	a.write(0xFF21, 0xF0)
	a.write(0xFF23, 0x80)
	if got := a.output(3); got != 0 {
		t.Errorf("noise with LFSR bit 0 set: %d, want 0", got)
	}
	a.ch[3].lfsr = 0x7FFE
	if got := a.output(3); got != 15 {
		t.Errorf("noise with LFSR bit 0 clear: %d, want 15", got)
	}

	a.write(0xFF12, 0x00) // DAC off disables the channel
	if got := a.output(0); got != 0 || a.ch[0].enabled {
		t.Errorf("channel 1 with DAC off: output %d enabled %v", got, a.ch[0].enabled)
	}
}

func TestAPUNoiseLFSR(t *testing.T) {
	g := newTestGB(t)
	a := g.APU
	a.write(0xFF21, 0xF0)
	a.write(0xFF22, 0x08) // narrow (7-bit) mode, fastest clock
	a.write(0xFF23, 0x80)
	seen := map[uint16]bool{}
	for range 1000 {
		a.clockChannels(a.period(3))
		seen[a.ch[3].lfsr&0x7F] = true
	}
	// A 7-bit LFSR cycles through 127 states.
	if len(seen) != 127 {
		t.Errorf("narrow LFSR visited %d states, want 127", len(seen))
	}
}

func TestAPUPanning(t *testing.T) {
	for _, c := range []struct {
		name        string
		nr51        byte
		left, right bool
	}{
		{"right only", 0x02, false, true},
		{"left only", 0x20, true, false},
	} {
		g := newTestGB(t, 0x18, 0xFE) // JR -2: spin forever
		g.Bus.write(0xFF24, 0x77)
		g.Bus.write(0xFF25, c.nr51)
		g.Bus.write(0xFF17, 0xF0)
		g.Bus.write(0xFF19, 0x87)
		g.RunFrame()
		var peakL, peakR int16
		s := g.APU.DrainSamples()
		for i := 0; i+1 < len(s); i += 2 {
			peakL, peakR = max(peakL, s[i]), max(peakR, s[i+1])
		}
		if (peakL > 1000) != c.left || (peakR > 1000) != c.right {
			t.Errorf("%s: peaks L=%d R=%d", c.name, peakL, peakR)
		}
	}
}

func TestAPUPower(t *testing.T) {
	g := newTestGB(t)
	a := g.APU
	a.write(0xFF17, 0xF0)
	a.write(0xFF19, 0x80)
	a.write(0xFF30, 0x12)

	a.write(0xFF26, 0x00)
	if got := a.read(0xFF26); got != 0x70 {
		t.Errorf("NR52 after power off %02X, want 70", got)
	}
	if got := a.read(0xFF17); got != 0x00 {
		t.Errorf("NR22 after power off %02X, want 00", got)
	}
	a.write(0xFF17, 0xF0)
	if got := a.read(0xFF17); got != 0x00 {
		t.Errorf("NR22 written while powered off: %02X", got)
	}
	a.write(0xFF30, 0x34) // wave RAM stays accessible
	if got := a.read(0xFF30); got != 0x34 {
		t.Errorf("wave RAM %02X, want 34", got)
	}

	a.write(0xFF26, 0x80)
	if got := a.read(0xFF26); got != 0xF0 {
		t.Errorf("NR52 after power on %02X, want F0", got)
	}
	a.write(0xFF11, 0x00)
	if got := a.read(0xFF11); got != 0x3F {
		t.Errorf("NR11 reads %02X, want 3F (length is write-only)", got)
	}
	if got := a.read(0xFF27); got != 0xFF {
		t.Errorf("unused 0xFF27 reads %02X, want FF", got)
	}
}

func TestToInt16(t *testing.T) {
	for _, c := range []struct {
		in   float64
		want int16
	}{
		{0, 0}, {0.5, 16383}, {2, 32767}, {-2, -32768},
	} {
		if got := toInt16(c.in); got != c.want {
			t.Errorf("toInt16(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

// Enabling the length counter when the next step of the frame sequencer
// does not clock it clocks it once (blargg's dmg_sound 03-trigger).
func TestAPULengthExtraClock(t *testing.T) {
	g := newTestGB(t)
	a := g.APU
	a.write(0xFF17, 0xF0) // channel 2 DAC on

	// Next step clocks the lengths: no extra clock.
	a.seqStep = 0
	a.write(0xFF16, 0x3E) // length 2
	a.write(0xFF19, 0xC0) // trigger, length enabled
	if a.ch[1].length != 2 {
		t.Errorf("length %d, want 2: no extra clock before a length step", a.ch[1].length)
	}

	// Next step does not clock them: enabling the counter clocks it.
	a.write(0xFF19, 0x80) // trigger, length disabled
	a.seqStep = 1
	a.write(0xFF19, 0x40) // length enabled, no trigger
	if a.ch[1].length != 1 || !a.ch[1].enabled {
		t.Errorf("length %d, enabled %v: want 1 and still playing", a.ch[1].length, a.ch[1].enabled)
	}
	a.write(0xFF19, 0x00)
	a.write(0xFF19, 0x40) // enabled again: the length reaches 0
	if a.ch[1].enabled {
		t.Error("channel 2 still playing after its length expired on the extra clock")
	}
	// Already enabled: no extra clock.
	a.write(0xFF16, 0x3E) // length 2
	a.write(0xFF19, 0xC0)
	a.write(0xFF19, 0x40)
	if a.ch[1].length != 2 {
		t.Errorf("length %d, want 2: the counter was already enabled", a.ch[1].length)
	}

	// Trigger with a length of 0 and the counter enabled: 63, not 64.
	a.write(0xFF16, 0x3F) // length 1
	a.write(0xFF19, 0x00)
	a.write(0xFF19, 0x40) // the extra clock: length 0
	a.write(0xFF19, 0xC0)
	if a.ch[1].length != 63 || !a.ch[1].enabled {
		t.Errorf("length %d after trigger, enabled %v: want 63 and playing", a.ch[1].length, a.ch[1].enabled)
	}
}

// The frame sequencer follows DIV: a step on every falling edge of bit 4 of
// DIV, so writing to DIV moves its next step.
func TestAPUSequencerFollowsDIV(t *testing.T) {
	g := newTestGB(t)
	a, tm := g.APU, g.Timer
	step := func() int { return a.seqStep }

	tm.counter = 0
	a.seqStep = 0
	for range 0x1000 / 4 { // up to bit 12 set: no falling edge yet
		tm.tick()
	}
	if step() != 0 {
		t.Fatalf("step %d before the first falling edge", step())
	}
	for range 0x1000 / 4 { // bit 12 falls once the counter reaches 0x2000
		tm.tick()
	}
	if step() != 1 {
		t.Fatalf("step %d after one falling edge, want 1", step())
	}

	// Writing to DIV with bit 12 set is a falling edge.
	tm.counter = 0x1000
	tm.write(0xFF04, 0)
	if step() != 2 {
		t.Errorf("step %d after resetting DIV with bit 4 set, want 2", step())
	}
	tm.write(0xFF04, 0)
	if step() != 2 {
		t.Errorf("step %d after resetting DIV with bit 4 clear, want 2", step())
	}

	// In double speed mode, bit 13 clocks it.
	g.Bus.doubleSpeed = true
	tm.counter = 0x1FFC
	tm.tick() // bit 12 falls, but not bit 13
	if step() != 2 {
		t.Errorf("step %d: bit 12 clocked the sequencer in double speed mode", step())
	}
	tm.counter = 0x3FFC
	tm.tick()
	if step() != 3 {
		t.Errorf("step %d: bit 13 did not clock the sequencer in double speed mode", step())
	}

	// Powered off, the APU ignores it.
	a.write(0xFF26, 0x00)
	tm.counter = 0x3FFC
	tm.tick()
	if step() != 3 {
		t.Error("the sequencer moved while the APU is off")
	}
}

// While the APU is powered off, a DMG still takes writes to the length
// counters (blargg's dmg_sound 08 and 11); a CGB ignores them.
func TestAPULengthWritesWhileOff(t *testing.T) {
	for _, c := range []struct {
		name string
		g    *GameBoy
		want [4]int
	}{
		{"dmg", newTestGB(t), [4]int{2, 3, 1, 4}},
		{"cgb", newTestCGB(t), [4]int{}},
	} {
		a := c.g.APU
		a.write(0xFF26, 0x00)
		a.write(0xFF11, 0xBE) // duty 2, length 2
		a.write(0xFF16, 0x3D)
		a.write(0xFF1B, 0xFF)
		a.write(0xFF20, 0x3C)
		got := [4]int{a.ch[0].length, a.ch[1].length, a.ch[2].length, a.ch[3].length}
		if got != c.want {
			t.Errorf("%s: lengths %v, want %v", c.name, got, c.want)
		}
		if a.regs[1] != 0 {
			t.Errorf("%s: NR11 %02X written while off", c.name, a.regs[1])
		}
		a.write(0xFF26, 0x80)
		if a.ch[0].length != c.want[0] {
			t.Errorf("%s: length %d after powering on, want %d", c.name, a.ch[0].length, c.want[0])
		}
	}
}

// playWave starts channel 3 on a wave RAM whose byte i holds i*0x11.
func playWave(g *GameBoy) *APU {
	a := g.APU
	for i := range uint16(16) {
		a.write(0xFF30+i, byte(i*0x11))
	}
	a.write(0xFF1A, 0x80) // DAC on
	a.write(0xFF1D, 0x00) // frequency 0x700: a byte every 512 T-cycles
	a.write(0xFF1E, 0x87) // trigger
	return a
}

// stepWave advances the APU 2 T-cycles at a time until channel 3 reads its
// next sample, and returns the position of that sample.
func stepWave(a *APU) byte {
	pos := a.ch[2].wavePos
	for a.ch[2].wavePos == pos {
		a.tick(2)
	}
	return a.ch[2].wavePos
}

// While channel 3 plays, the wave RAM accesses reach the byte it reads: at
// any time on a CGB, only as it reads it on a DMG (blargg's 09 and 12).
func TestAPUWaveRAMWhilePlaying(t *testing.T) {
	a := playWave(newTestCGB(t))
	pos := stepWave(a)
	pos = stepWave(a)
	a.tick(100)
	if got, want := a.read(0xFF3F), byte(pos/2*0x11); got != want {
		t.Errorf("CGB: read %02X at sample %d, want %02X", got, pos, want)
	}
	a.write(0xFF30, 0xAB)
	if a.regs[0x20+pos/2] != 0xAB || a.regs[0x20] == 0xAB {
		t.Error("CGB: the write did not reach the byte being played")
	}

	a = playWave(newTestGB(t))
	stepWave(a)
	pos = stepWave(a)
	if got, want := a.read(0xFF3F), byte(pos/2*0x11); got != want {
		t.Errorf("DMG: read %02X as the channel reads sample %d, want %02X", got, pos, want)
	}
	a.tick(2)
	if got := a.read(0xFF3F); got != 0xFF {
		t.Errorf("DMG: read %02X between two reads of the channel, want FF", got)
	}
	a.write(0xFF3F, 0xAB)
	for i, v := range a.regs[0x20:0x30] {
		if v != byte(i*0x11) {
			t.Errorf("DMG: write between two reads changed byte %d to %02X", i, v)
		}
	}

	a.write(0xFF1A, 0x00) // channel 3 off: plain accesses again
	if got := a.read(0xFF35); got != 0x55 {
		t.Errorf("read %02X with channel 3 off, want 55", got)
	}
}

// On a DMG, retriggering channel 3 just before it reads a byte copies the
// four bytes around it over the first four (blargg's dmg_sound 10).
func TestAPUWaveTriggerCorruption(t *testing.T) {
	for _, c := range []struct {
		name    string
		g       *GameBoy
		corrupt bool
	}{{"dmg", newTestGB(t), true}, {"cgb", newTestCGB(t), false}} {
		a := playWave(c.g)
		for a.ch[2].wavePos < 9 {
			stepWave(a)
		}
		next := int(a.ch[2].wavePos+1) / 2 // the byte read next: 5
		for a.ch[2].timer-a.pending > 2 {
			a.tick(2)
		}
		a.write(0xFF1E, 0x87)
		for i := range 4 {
			want := byte(i * 0x11)
			if c.corrupt {
				want = byte((next&^3 + i) * 0x11)
			}
			if got := a.regs[0x20+i]; got != want {
				t.Errorf("%s: byte %d is %02X after the trigger, want %02X", c.name, i, got, want)
			}
		}
	}
}
