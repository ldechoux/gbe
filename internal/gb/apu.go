package gb

import "math"

// ClockRate is the DMG master clock in Hz.
const ClockRate = 4194304

// Read-back masks for NR10..NR52: unused and write-only bits read as 1.
var apuReadMask = [0x17]byte{
	0x80, 0x3F, 0x00, 0xFF, 0xBF, // NR10-NR14
	0xFF, 0x3F, 0x00, 0xFF, 0xBF, // NR20-NR24
	0x7F, 0xFF, 0x9F, 0xFF, 0xBF, // NR30-NR34
	0xFF, 0xFF, 0x00, 0x00, 0xBF, // NR40-NR44
	0x00, 0x00, 0x70, // NR50-NR52
}

var dutyTable = [4][8]byte{
	{0, 0, 0, 0, 0, 0, 0, 1},
	{1, 0, 0, 0, 0, 0, 0, 1},
	{1, 0, 0, 0, 0, 1, 1, 1},
	{0, 1, 1, 1, 1, 1, 1, 0},
}

var noiseDivisors = [8]int{8, 16, 32, 48, 64, 80, 96, 112}

type envelope struct {
	initial byte
	up      bool
	period  byte
	volume  byte
	timer   byte
}

func (e *envelope) load(v byte) {
	e.initial, e.up, e.period = v>>4, v&0x08 != 0, v&7
}

func (e *envelope) trigger() {
	e.volume = e.initial
	e.timer = e.period
	if e.timer == 0 {
		e.timer = 8
	}
}

func (e *envelope) step() {
	if e.period == 0 {
		return
	}
	e.timer--
	if e.timer > 0 {
		return
	}
	e.timer = e.period
	if e.up && e.volume < 15 {
		e.volume++
	} else if !e.up && e.volume > 0 {
		e.volume--
	}
}

type channel struct {
	enabled   bool
	dac       bool
	length    int
	lengthOn  bool
	freq      int
	timer     int
	env       envelope
	duty      byte // square
	dutyPos   byte
	wavePos   byte   // wave: sample played, its byte is the last one read
	waveShift byte   // wave: 0=mute(4), 1=100%, 2=50%, 3=25%
	waveRead  bool   // wave: a byte was read since the trigger
	reload    int    // wave: timer value at the last read
	lfsr      uint16 // noise
	narrow    bool

	// Sweep (channel 1 only).
	sweepPeriod  byte
	sweepNegate  bool
	sweepShift   byte
	sweepTimer   byte
	sweepOn      bool
	sweepShadow  int
	sweepNegUsed bool
}

func (ch *channel) lengthStep() {
	if ch.lengthOn && ch.length > 0 {
		ch.length--
		if ch.length == 0 {
			ch.enabled = false
		}
	}
}

// APU emulates the four sound channels and mixes them into interleaved
// stereo int16 samples.
type APU struct {
	bus  *Bus
	regs [0x30]byte // raw register writes, 0xFF10-0xFF3F
	on   bool

	ch [4]channel

	// The frame sequencer, clocked by DIV (see divEvent). seqStep is its
	// next step: lengths on the even ones, sweep on 2 and 6, envelopes on 7.
	seqStep int

	// The channel timers are only brought up to date when one of them
	// expires, or before anything else uses them: pending T-cycles are not
	// applied yet, and the first timer expires in untilClock T-cycles.
	pending    int
	untilClock int

	samplePeriod float64 // T-cycles per output sample
	phaseScale   float64 // blipPhases / samplePeriod
	sampleClock  float64 // T-cycles since the last output sample
	blip         blip    // the mixed output, band-limited (see blip.go)
	hpL, hpR     float64
	hpCharge     float64
	samples      []int16

	// The mixed output only changes now and then (a step of a waveform, a
	// register write...), so it is computed again only after a change.
	mixValid bool
}

func newAPU(bus *Bus) *APU {
	a := &APU{bus: bus}
	a.SetSampleRate(48000)
	return a
}

// SetSampleRate changes the output rate. Frontends may nudge it slightly to
// keep the audio buffer level stable.
func (a *APU) SetSampleRate(rate float64) {
	a.samplePeriod = ClockRate / rate
	a.phaseScale = blipPhases / a.samplePeriod
	a.hpCharge = math.Pow(0.999958, ClockRate/rate)
}

// DrainSamples returns the samples generated since the last call. The
// returned slice is only valid until the next emulation step.
func (a *APU) DrainSamples() []int16 {
	s := a.samples
	a.samples = a.samples[:0]
	return s
}

func (a *APU) read(addr uint16) byte {
	off := addr - 0xFF10
	switch {
	case addr == 0xFF26:
		v := byte(0x70)
		if a.on {
			v |= 0x80
		}
		for i := range 4 {
			if a.ch[i].enabled {
				v |= 1 << i
			}
		}
		return v
	case addr < 0xFF27:
		return a.regs[off] | apuReadMask[off]
	case addr < 0xFF30:
		return 0xFF
	}
	i, ok := a.waveRAMIndex(addr)
	if !ok {
		return 0xFF
	}
	return a.regs[0x20+i]
}

// waveRAMIndex returns the byte of the wave RAM that an access to addr
// reaches. While channel 3 plays, it is the byte the channel last read, on
// a CGB, and on a DMG only at the very moment it reads it (2 T-cycles):
// otherwise reads return 0xFF and writes are lost.
func (a *APU) waveRAMIndex(addr uint16) (int, bool) {
	w := &a.ch[2]
	if !w.enabled {
		return int(addr - 0xFF30), true
	}
	if !a.bus.cgb && !a.waveJustRead() {
		return 0, false
	}
	return int(w.wavePos / 2), true
}

// corruptWaveRAM is a DMG bug: retriggering channel 3 as it is about to
// read a byte of the wave RAM (in the next 2 T-cycles) overwrites the first
// byte with it, or the first four bytes with the four it belongs to.
func (a *APU) corruptWaveRAM() {
	wave := a.regs[0x20:0x30]
	next := int((a.ch[2].wavePos+1)/2) & 0xF
	if next < 4 {
		wave[0] = wave[next]
	} else {
		copy(wave[:4], wave[next&^3:])
	}
}

// waveJustRead reports whether channel 3 read a byte of the wave RAM in the
// last 2 T-cycles (one cycle of its 2 MHz clock).
func (a *APU) waveJustRead() bool {
	w := &a.ch[2]
	since := w.reload - (w.timer - a.pending)
	return w.waveRead && since < waveReadWindow
}

const waveReadWindow = 2

func (a *APU) write(addr uint16, v byte) {
	a.mixValid = false
	a.catchUp()
	a.untilClock = 0 // the write may change a timer: check on the next tick
	off := addr - 0xFF10
	if addr >= 0xFF30 {
		if i, ok := a.waveRAMIndex(addr); ok {
			a.regs[0x20+i] = v
		}
		return
	}
	if addr == 0xFF26 {
		if a.on && v&0x80 == 0 {
			// Powering off clears every register and silences all channels.
			clear(a.regs[:0x16])
			a.ch = [4]channel{}
		}
		if !a.on && v&0x80 != 0 {
			a.seqStep = 0
		}
		a.on = v&0x80 != 0
		return
	}
	if !a.on {
		// The registers are read-only while powered off, except for the
		// length counters on a DMG.
		if !a.bus.cgb {
			switch addr {
			case 0xFF11, 0xFF16:
				a.ch[(addr-0xFF11)/5].length = 64 - int(v&0x3F)
			case 0xFF1B:
				a.ch[2].length = 256 - int(v)
			case 0xFF20:
				a.ch[3].length = 64 - int(v&0x3F)
			}
		}
		return
	}
	a.regs[off] = v

	switch addr {
	// Channel 1
	case 0xFF10:
		c := &a.ch[0]
		c.sweepPeriod = (v >> 4) & 7
		c.sweepNegate = v&0x08 != 0
		c.sweepShift = v & 7
		if !c.sweepNegate && c.sweepNegUsed {
			c.enabled = false
		}
	case 0xFF11:
		a.ch[0].duty = v >> 6
		a.ch[0].length = 64 - int(v&0x3F)
	case 0xFF12:
		a.writeEnvelope(0, v)
	case 0xFF13:
		a.ch[0].freq = a.ch[0].freq&0x700 | int(v)
	case 0xFF14:
		a.writeControl(0, v, 64)
	// Channel 2
	case 0xFF16:
		a.ch[1].duty = v >> 6
		a.ch[1].length = 64 - int(v&0x3F)
	case 0xFF17:
		a.writeEnvelope(1, v)
	case 0xFF18:
		a.ch[1].freq = a.ch[1].freq&0x700 | int(v)
	case 0xFF19:
		a.writeControl(1, v, 64)
	// Channel 3
	case 0xFF1A:
		a.ch[2].dac = v&0x80 != 0
		if !a.ch[2].dac {
			a.ch[2].enabled = false
		}
	case 0xFF1B:
		a.ch[2].length = 256 - int(v)
	case 0xFF1C:
		a.ch[2].waveShift = (v >> 5) & 3
	case 0xFF1D:
		a.ch[2].freq = a.ch[2].freq&0x700 | int(v)
	case 0xFF1E:
		a.writeControl(2, v, 256)
	// Channel 4
	case 0xFF20:
		a.ch[3].length = 64 - int(v&0x3F)
	case 0xFF21:
		a.writeEnvelope(3, v)
	case 0xFF22:
		a.ch[3].narrow = v&0x08 != 0
	case 0xFF23:
		a.writeControl(3, v, 64)
	}
}

func (a *APU) writeEnvelope(i int, v byte) {
	c := &a.ch[i]
	c.env.load(v)
	c.dac = v&0xF8 != 0
	if !c.dac {
		c.enabled = false
	}
}

func (a *APU) period(i int) int {
	c := &a.ch[i]
	switch i {
	case 2:
		return (2048 - c.freq) * 2
	case 3:
		nr43 := a.regs[0x12]
		return noiseDivisors[nr43&7] << (nr43 >> 4)
	}
	return (2048 - c.freq) * 4
}

func (a *APU) writeControl(i int, v byte, maxLen int) {
	c := &a.ch[i]
	if i != 3 {
		c.freq = c.freq&0xFF | int(v&7)<<8
	}
	wasOn := c.lengthOn
	c.lengthOn = v&0x40 != 0
	// When the next step of the frame sequencer does not clock the lengths,
	// enabling the length counter clocks it once: it may expire right away.
	noLengthStep := a.seqStep&1 != 0
	if noLengthStep && !wasOn && c.lengthOn && c.length > 0 {
		c.length--
		if c.length == 0 && v&0x80 == 0 {
			c.enabled = false
		}
	}
	if v&0x80 == 0 {
		return
	}
	// Trigger.
	if i == 2 && c.enabled && !a.bus.cgb && c.timer <= 2 {
		a.corruptWaveRAM()
	}
	c.enabled = c.dac
	if c.length == 0 {
		c.length = maxLen
		if c.lengthOn && noLengthStep {
			c.length-- // the extra clock again
		}
	}
	c.timer = a.period(i)
	switch i {
	case 0:
		c.env.trigger()
		c.sweepShadow = c.freq
		c.sweepTimer = c.sweepPeriod
		if c.sweepTimer == 0 {
			c.sweepTimer = 8
		}
		c.sweepOn = c.sweepPeriod != 0 || c.sweepShift != 0
		c.sweepNegUsed = false
		if c.sweepShift != 0 {
			a.sweepCalc()
		}
	case 1:
		c.env.trigger()
	case 2:
		// The first byte is read 3 cycles of the 2 MHz clock later than
		// the period, from the second sample on.
		c.timer += 6
		c.wavePos = 0
		c.waveRead = false
	case 3:
		c.env.trigger()
		c.lfsr = 0x7FFF
	}
}

func (a *APU) sweepCalc() int {
	c := &a.ch[0]
	delta := c.sweepShadow >> c.sweepShift
	var f int
	if c.sweepNegate {
		f = c.sweepShadow - delta
		c.sweepNegUsed = true
	} else {
		f = c.sweepShadow + delta
	}
	if f > 2047 {
		c.enabled = false
	}
	return f
}

func (a *APU) sweepStep() {
	c := &a.ch[0]
	c.sweepTimer--
	if c.sweepTimer > 0 {
		return
	}
	c.sweepTimer = c.sweepPeriod
	if c.sweepTimer == 0 {
		c.sweepTimer = 8
	}
	if !c.sweepOn || c.sweepPeriod == 0 {
		return
	}
	f := a.sweepCalc()
	if f <= 2047 && c.sweepShift != 0 {
		c.sweepShadow = f
		c.freq = f
		a.regs[3] = byte(f)
		a.regs[4] = a.regs[4]&^7 | byte(f>>8)&7
		a.sweepCalc()
	}
}

// divEvent is the clock of the frame sequencer: a falling edge of bit 12
// of the system counter (bit 4 of DIV), or of bit 13 in double speed mode,
// 512 times per second. The Timer calls it, also when a write to DIV or
// STOP resets the counter.
func (a *APU) divEvent() {
	if a.on {
		a.sequencerStep()
		a.mixValid = false
	}
}

func (a *APU) sequencerStep() {
	switch a.seqStep {
	case 0, 4:
		a.lengthSteps()
	case 2, 6:
		a.lengthSteps()
		a.sweepStep()
	case 7:
		a.ch[0].env.step()
		a.ch[1].env.step()
		a.ch[3].env.step()
	}
	a.seqStep = (a.seqStep + 1) & 7
}

func (a *APU) lengthSteps() {
	for i := range a.ch {
		a.ch[i].lengthStep()
	}
}

// tick advances the APU by the given number of T-cycles at the normal speed
// (4 per M-cycle, 2 in CGB double speed mode).
func (a *APU) tick(cycles int) {
	if a.on {
		a.pending += cycles
		if a.pending >= a.untilClock {
			a.clockChannels(a.pending)
			a.pending = 0
			a.untilClock = min(a.ch[0].timer, a.ch[1].timer, a.ch[2].timer, a.ch[3].timer)
		}
	}
	a.mix(cycles)
}

// catchUp applies the pending T-cycles to the channel timers. None of them
// expires, so the channels stay where they are.
func (a *APU) catchUp() {
	if a.pending > 0 {
		a.clockChannels(a.pending)
		a.untilClock -= a.pending
		a.pending = 0
	}
}

func (a *APU) clockChannels(cycles int) {
	for i := range 2 {
		c := &a.ch[i]
		c.timer -= cycles
		for c.timer <= 0 {
			c.timer += a.period(i)
			c.dutyPos = (c.dutyPos + 1) & 7
			a.outputChanged(c)
		}
	}
	w := &a.ch[2]
	w.timer -= cycles
	for w.timer <= 0 {
		w.reload = a.period(2)
		w.timer += w.reload
		w.wavePos = (w.wavePos + 1) & 31
		w.waveRead = true
		a.outputChanged(w)
	}
	n := &a.ch[3]
	n.timer -= cycles
	for n.timer <= 0 {
		n.timer += a.period(3)
		bit := (n.lfsr ^ n.lfsr>>1) & 1
		n.lfsr = n.lfsr>>1 | bit<<14
		if n.narrow {
			n.lfsr = n.lfsr&^(1<<6) | bit<<6
		}
		a.outputChanged(n)
	}
}

// outputChanged is called when the waveform of a channel moves on, which
// changes the mix if the channel is heard.
func (a *APU) outputChanged(c *channel) {
	if c.enabled && c.dac {
		a.mixValid = false
	}
}

// output returns the digital (0-15) output of channel i.
func (a *APU) output(i int) byte {
	c := &a.ch[i]
	if !c.enabled {
		return 0
	}
	switch i {
	case 0, 1:
		return dutyTable[c.duty][c.dutyPos] * c.env.volume
	case 2:
		b := a.regs[0x20+c.wavePos/2]
		if c.wavePos&1 == 0 {
			b >>= 4
		}
		b &= 0x0F
		if c.waveShift == 0 {
			return 0
		}
		return b >> (c.waveShift - 1)
	}
	return byte(^c.lfsr&1) * c.env.volume
}

// mixScale is the largest mixed level (mixChannels) left or right: 4
// channels at 15 times the largest master volume, 8.
const mixScale = 4 * 15 * 8

func (a *APU) mix(cycles int) {
	if !a.mixValid {
		// The new level starts now: where in the output sample being made.
		phase := min(int(a.sampleClock*a.phaseScale+0.5), blipPhases)
		l, r := a.mixChannels()
		a.blip.set(l, r, phase)
		a.mixValid = true
	}

	a.sampleClock += float64(cycles)
	if a.sampleClock < a.samplePeriod {
		return
	}
	a.sampleClock -= a.samplePeriod
	bl, br := a.blip.next()
	l := float64(bl) / (mixScale << blipShift)
	r := float64(br) / (mixScale << blipShift)

	// High-pass filter removing the DC offset, like the real hardware. The
	// explicit conversions round the products, which some architectures
	// (arm64) would otherwise fuse with the subtractions: the output is the
	// same everywhere.
	outL := l - a.hpL
	a.hpL = l - float64(outL*a.hpCharge)
	outR := r - a.hpR
	a.hpR = r - float64(outR*a.hpCharge)

	a.samples = append(a.samples, toInt16(outL), toInt16(outR))
}

// mixChannels returns the analog output of the left and right channels,
// from -mixScale to mixScale.
func (a *APU) mixChannels() (l, r int) {
	if !a.on {
		return 0, 0
	}
	nr51 := a.regs[0x15]
	for i := range 4 {
		if !a.ch[i].dac {
			continue
		}
		// DAC: 0..15 -> +15..-15 (+1..-1)
		v := 15 - 2*int(a.output(i))
		if nr51&(0x10<<i) != 0 {
			l += v
		}
		if nr51&(1<<i) != 0 {
			r += v
		}
	}
	nr50 := a.regs[0x14]
	l *= int((nr50>>4)&7 + 1)
	r *= int(nr50&7 + 1)
	return l, r
}

func toInt16(v float64) int16 {
	v *= 32767
	if v > 32767 {
		return 32767
	}
	if v < -32768 {
		return -32768
	}
	return int16(v)
}
