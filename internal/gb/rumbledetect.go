package gb

import (
	"math"
	"math/bits"
)

// Games without a rumble motor can still shake the gamepad: the shocks of a
// game (explosions, hits, falls) come with typical sound effects, which the
// rumble detector looks for. It follows the notes as the game plays them,
// each time a channel is triggered, rather than the level of the channels
// once per frame:
//
//   - Music or sound effect. Games play their music and their sound effects
//     on the same four channels: an effect takes a channel from the music
//     while it lasts. The music is regular and plays a few instruments over
//     and over, while effects come when the player acts. Three hints tell
//     them apart: the code that writes the note, when the game has one
//     routine for its music and another for its effects; how regularly the
//     same note comes back; and how often it was heard.
//   - Shock. Only effects are scored: loud, low and long noise on channel 4
//     (an explosion, a hit), noise that gets lower as it plays, and fast
//     frequency sweeps on channel 1, a falling one more than a rising one.
//   - Pulse. A shock makes a pulse that starts at once and fades with the
//     envelope of its channel, held long enough for a motor to be felt.
//
// The detector only watches: it is not part of the state of the console, so
// the emulation, the sound and the save states do not change.

// RumbleNote is a note as the rumble detector judged it. Notes of channels
// 2 and 3, which play the melody, are always music.
type RumbleNote struct {
	Frame   float64 // when it was triggered, in frames since power on
	Channel int     // 1 to 4
	Regs    [5]byte // NRx0 to NRx4 when it was triggered
	PC      uint16  // after the instruction that triggered it
	Music   float64 // how likely it is music, from 0 to 1
	Shock   float64 // how hard it shakes, from 0 to 1, after Music
}

// RumbleParams are the settings of the rumble detector a player would feel,
// to tune it (see DefaultRumbleParams).
type RumbleParams struct {
	Floor    float64 `json:"floor"`    // a weaker shock, from 0 to 1, does not shake
	Gain     float64 `json:"gain"`     // shocks are scored, then multiplied by it
	Music    float64 `json:"music"`    // a note this likely music, from 0 to 1, never shakes
	Duration float64 `json:"duration"` // how long pulses last, 1 for pulseMinHold to pulseMaxHold
	Sweeps   float64 `json:"sweeps"`   // weight of the channel 1 sweeps against the noise
	// Steady is how many frames a note of steady volume is taken to last,
	// at Duration 1: games that fade their sounds themselves trigger it
	// again and again, lower each time (Street Fighter Alpha, Zelda).
	Steady float64 `json:"steady"`
	// Shake is how hard a screen shaking at full strength shakes, from 0
	// (never) to 1 (see rumblescreen.go).
	Shake float64 `json:"shake"`
	// Flash makes the shocks heard as the screen flashes stronger: by this
	// share of their strength.
	Flash float64 `json:"flash"`
}

// DefaultRumbleParams are the settings the detector runs with unless told
// otherwise.
func DefaultRumbleParams() RumbleParams {
	// Tuned by feel on recorded play of Donkey Kong Country, Kirby's Dream
	// Land 2, Mega Man Xtreme 2, R-Type DX, Street Fighter Alpha, Super
	// Mario Land, Tetris, Tetris DX, Wario Land 3, Zelda: Link's Awakening
	// and the storm of its DX intro (gbe -rumble-tune). The sweeps weigh as
	// much as the noise: the blows of Super Mario Land are sweeps.
	return RumbleParams{Floor: 0.245, Gain: 0.7, Music: 0.5, Duration: 1, Sweeps: 1, Steady: 8, Shake: 0.6, Flash: 1}
}

// SetRumbleParams changes the settings of the rumble detector.
func (g *GameBoy) SetRumbleParams(p RumbleParams) {
	g.rumbleParams = p
	g.rumbleTuned = true
}

// Detector settings, in frames and from 0 to 1.
const (
	sigHalfLife   = 30 * 60 // how fast notes heard long ago are forgotten
	regularTol    = 0.75    // how far from a former interval a regular one may be
	regularShare  = 0.03    // or, for long ones, this share of it
	beatMax       = 2 * 60  // frames between the notes of a beat at most
	pulseMinHold  = 4       // frames a pulse lasts at least (about 67 ms), at Duration 1
	pulseMaxHold  = 30      // and at most (half a second)
	burstPerFrame = 0.5     // notes of one kind per frame beyond which they are a buzz, not shocks
	maxLowered    = 2       // times a noise getting lower makes its pulse stronger at most
)

// noteSig identifies an instrument: the registers of a note but its pitch on
// channel 1 (the music plays one instrument at many pitches).
type noteSig struct {
	ch   int
	regs [5]byte
}

// sigStats is what the detector knows of one instrument.
type sigStats struct {
	heard     float64    // times heard, fading with sigHalfLife
	last      float64    // frame it was last triggered
	intervals [6]float64 // between its last triggers, the newest first
	burst     float64    // triggers in the last few frames, fading fast
}

// writer is the code that writes notes: the channels it triggered.
type writer struct{ channels byte }

// pulse is the shaking of a shock on a channel.
type pulse struct {
	strength float64 // at the start
	start    float64 // frame
	hold     float64 // frames it lasts at most
	volume   byte    // of the envelope when triggered
	active   bool
	lowness  float64 // of the noise, on channel 4 (see lower)
	lowered  int     // times the noise got lower
}

type rumbleDetector struct {
	g       *GameBoy
	sigs    map[noteSig]*sigStats
	writers map[uint16]*writer
	pulses  [4]pulse
	level   float64 // of the last frame
	lastSig [4]noteSig
	trace   []RumbleNote // nil unless traced
	tracing bool
	screen  screenWatch
}

// GuessRumble turns the rumble detector on or off. While on, GuessedRumble
// tells how hard the game would shake.
func (g *GameBoy) GuessRumble(on bool) {
	switch {
	case on && g.guess == nil:
		g.guess = &rumbleDetector{g: g, sigs: map[noteSig]*sigStats{}, writers: map[uint16]*writer{}}
		g.guess.restart()
		g.guess.attach()
	case !on && g.guess != nil:
		g.APU.watch = nil
		g.guess = nil
	}
}

// GuessedRumble returns how hard the last frame would shake, from 0 to 1, as
// guessed from its sound effects (see GuessRumble). It is 0 while the
// detector is off.
func (g *GameBoy) GuessedRumble() float64 {
	if g.guess == nil {
		return 0
	}
	return g.guess.level
}

// TraceRumble keeps the notes the detector judges, for DrainRumbleNotes.
func (g *GameBoy) TraceRumble(on bool) {
	if g.guess != nil {
		g.guess.tracing = on
		g.guess.trace = g.guess.trace[:0]
	}
}

// DrainRumbleNotes returns the notes judged since the last call, when traced.
func (g *GameBoy) DrainRumbleNotes() []RumbleNote {
	if g.guess == nil {
		return nil
	}
	notes := g.guess.trace
	g.guess.trace = nil
	return notes
}

func (d *rumbleDetector) attach() { d.g.APU.watch = d.write }

// RumbleMemory is what the rumble detector learned of a game (see
// RumbleMemory).
type RumbleMemory struct {
	sigs    map[noteSig]sigStats
	writers map[uint16]writer
}

// RumbleMemory copies what the rumble detector learned of the game, for
// SetRumbleMemory: with Snapshot and Restore, the same moment can be
// played again with the detector in the same mind.
func (g *GameBoy) RumbleMemory() RumbleMemory {
	m := RumbleMemory{sigs: map[noteSig]sigStats{}, writers: map[uint16]writer{}}
	if d := g.guess; d != nil {
		for k, s := range d.sigs {
			m.sigs[k] = *s
		}
		for k, w := range d.writers {
			m.writers[k] = *w
		}
	}
	return m
}

// SetRumbleMemory gives the rumble detector what it learned at the time
// of RumbleMemory, and forgets its pulses. Call it after Restore.
func (g *GameBoy) SetRumbleMemory(m RumbleMemory) {
	d := g.guess
	if d == nil {
		return
	}
	d.restart()
	d.sigs, d.writers = map[noteSig]*sigStats{}, map[uint16]*writer{}
	for k, s := range m.sigs {
		d.sigs[k] = &s
	}
	for k, w := range m.writers {
		d.writers[k] = &w
	}
}

// params are the settings the detector runs with.
func (d *rumbleDetector) params() RumbleParams {
	if d.g.rumbleTuned {
		return d.g.rumbleParams
	}
	return DefaultRumbleParams()
}

// minHold and maxHold are how long pulses last at least and at most, in
// frames.
func (d *rumbleDetector) minHold() float64 { return pulseMinHold * d.params().Duration }
func (d *rumbleDetector) maxHold() float64 { return pulseMaxHold * d.params().Duration }

// restart forgets the pulses and when each note was last heard, after the
// machine jumped in time (rewind, state loaded). What was learned of the
// game is kept.
func (d *rumbleDetector) restart() {
	d.pulses = [4]pulse{}
	d.level = 0
	d.screen = screenWatch{flashAt: math.Inf(-1)}
	for _, s := range d.sigs {
		s.last, s.burst = math.Inf(-1), 0
	}
}

func (d *rumbleDetector) now() float64 {
	return float64(d.g.Bus.cycles) / CyclesPerFrame
}

// write sees a write to a sound register.
func (d *rumbleDetector) write(addr uint16, v byte) {
	switch addr {
	case 0xFF14, 0xFF19, 0xFF1E, 0xFF23:
		if v&0x80 != 0 {
			d.trigger(int(addr-0xFF14) / 5)
		}
	case 0xFF22:
		d.lower()
	}
}

// trigger judges a note of channel i (0 to 3) just triggered.
func (d *rumbleDetector) trigger(i int) {
	pc := d.g.CPU.pc
	w := d.writers[pc]
	if w == nil {
		w = &writer{}
		d.writers[pc] = w
	}
	w.channels |= 1 << i
	a := d.g.APU
	t := d.now()
	if i == 1 || i == 2 {
		// Channels 2 and 3 play the melody: never shocks.
		if d.tracing {
			n := RumbleNote{Frame: t, Channel: i + 1, PC: pc, Music: 1}
			copy(n.Regs[:], a.regs[i*5:i*5+5])
			d.trace = append(d.trace, n)
		}
		return
	}
	sig := noteSig{ch: i}
	copy(sig.regs[:], a.regs[i*5:i*5+5])
	sig.regs[4] &= 0x40 // the trigger and, on channel 1, the pitch
	if i == 0 {
		sig.regs[3] = 0
		sig.regs[4] = 0
	}
	s := d.sigs[sig]
	if s == nil {
		s = &sigStats{last: math.Inf(-1)}
		d.sigs[sig] = s
	}
	music := d.music(pc, i, s, t)
	shock := 0.0
	p := d.params()
	if music < p.Music { // likely music: never a shock
		shock = math.Min(1, p.Gain*d.shock(i)) * (1 - music)
	}
	// The same note again and again, every frame or so, is a buzz.
	if s.burst > burstPerFrame*4 {
		shock *= burstPerFrame * 4 / s.burst
	}
	s.remember(t)
	d.lastSig[i] = sig
	if shock >= p.Floor && shock > 0 {
		if t-d.screen.flashAt <= flashBoostTime {
			shock = math.Min(1, shock*(1+p.Flash)) // the screen flashed just before
		}
		c := &a.ch[i]
		d.pulses[i] = pulse{strength: shock, start: t, hold: d.duration(i), volume: c.env.volume, active: true}
		if i == 3 {
			d.pulses[i].lowness = noiseLowness(a.regs[0x12])
		}
	} else if d.pulses[i].active {
		d.pulses[i].active = false // another note took the channel
	}
	if d.tracing {
		n := RumbleNote{Frame: t, Channel: i + 1, PC: pc, Music: music, Shock: shock}
		copy(n.Regs[:], a.regs[i*5:i*5+5])
		d.trace = append(d.trace, n)
	}
}

// remember adds a trigger of the instrument at frame t.
func (s *sigStats) remember(t float64) {
	dt := t - s.last
	if !math.IsInf(dt, 1) {
		copy(s.intervals[1:], s.intervals[:])
		s.intervals[0] = dt
		s.heard *= math.Exp2(-dt / sigHalfLife)
		s.burst *= math.Exp2(-dt / 2) // half-life of 2 frames
	}
	s.heard++
	s.burst++
	s.last = t
}

// music tells how likely a note of channel i, written by the code at pc,
// is music, from 0 to 1.
func (d *rumbleDetector) music(pc uint16, i int, s *sigStats, t float64) float64 {
	m := 0.0
	// The writer. When some code writes the melody of channel 2 and this
	// channel, and other code writes this channel but not channel 2, the
	// game may have a routine for its music and one for its effects. The
	// first one is a hint of music. The second is a hint of effects only
	// when it writes several channels: some games write each channel with
	// its own routine, music and effects alike (the drums of Wario Land 3),
	// but an effect engine plays the noise and the sweeps alike (Super
	// Mario Land, Tetris, Zelda). Channel 3 tells nothing: its wave plays
	// effects too (Zelda).
	const melody = 1 << 1
	musicWriter, effectWriter := false, false
	for _, w := range d.writers {
		switch {
		case w.channels&melody != 0 && w.channels&(1<<i) != 0:
			musicWriter = true
		case w.channels&melody == 0 && w.channels&(1<<i) != 0:
			effectWriter = true
		}
	}
	if musicWriter && effectWriter {
		switch w := d.writers[pc].channels; {
		case w&melody != 0:
			m += 0.5
		case bits.OnesCount8(w) >= 2:
			m -= 0.4
		}
	}
	// Regular: the time since the note was last heard is a multiple, or a
	// fraction, of a former interval, as on a beat. The tempo of a timer
	// driven music is no whole number of frames: the tolerance grows with
	// the interval. A beat comes back within beatMax frames: effects heard
	// seconds apart (enemies stomped in Super Mario Land) match a former
	// interval by chance.
	if dt := t - s.last; dt >= 2 && dt <= beatMax {
		beats := 0
		for _, iv := range s.intervals {
			if iv < 2 || iv > beatMax {
				continue
			}
			long, short := math.Max(dt, iv), math.Min(dt, iv)
			r := math.Round(long / short)
			if r <= 4 && math.Abs(long-r*short) < math.Max(regularTol, regularShare*long) {
				beats++
			}
		}
		// One match may be chance; two make a beat.
		m += 0.25 * math.Min(2, float64(beats))
	}
	// Familiar: heard many times lately.
	m += 0.3 * (1 - math.Exp(-s.heard/6))
	return math.Max(0, math.Min(1, m))
}

// shock tells how hard the note just triggered on channel i (0 or 3) would
// shake, from 0 to 1, were it an effect.
func (d *rumbleDetector) shock(i int) float64 {
	a := d.g.APU
	c := &a.ch[i]
	if !c.enabled {
		return 0
	}
	loud := float64(c.env.volume) / 15 * d.output(i)
	if c.env.up {
		loud *= 0.6 // swelling: not a blow
	}
	long := math.Min(1, d.duration(i)/12)
	switch i {
	case 3:
		return loud * (0.1 + 0.9*noiseLowness(a.regs[0x12])) * (0.4 + 0.6*long)
	default:
		nr10 := a.regs[0x00]
		shift, pace := float64(nr10&7), float64(nr10>>4&7)
		if shift == 0 || pace == 0 {
			return 0 // a plain note
		}
		// Octaves per second: each sweep step, every pace/128 s, changes
		// the frequency by one part in 2^shift.
		rate := math.Log2(1+math.Exp2(-shift)) * 128 / pace
		fast := math.Max(0, math.Min(1, (math.Log2(rate)+1)/6))
		dir := 1.0
		if nr10&0x08 == 0 {
			dir = 0.6 // rising: jumps and coins more often than blows
		}
		return d.params().Sweeps * loud * fast * dir * (0.4 + 0.6*long)
	}
}

// noiseLowness tells how low the noise of NR43 sounds, from 0 (a hiss, its
// shift register clocked at 32 kHz or more) to 1 (a rumble, at 2 kHz or
// less).
func noiseLowness(nr43 byte) float64 {
	r := float64(nr43 & 7)
	if r == 0 {
		r = 0.5
	}
	hz := 262144 / r / math.Exp2(float64(nr43>>4))
	l := (15 - math.Log2(hz)) / 4
	if nr43&0x08 != 0 {
		l += 0.2 // the short register buzzes lower than its clock
	}
	return math.Max(0, math.Min(1, l))
}

// output tells how loud channel i comes out, from 0 to 1: the master volume
// of the sides it is sent to.
func (d *rumbleDetector) output(i int) float64 {
	a := d.g.APU
	nr50, nr51 := a.regs[0x14], a.regs[0x15]
	v := 0.0
	if nr51>>i&1 != 0 {
		v += float64(nr50&7+1) / 16
	}
	if nr51>>(i+4)&1 != 0 {
		v += float64(nr50>>4&7+1) / 16
	}
	return math.Sqrt(v) // a quiet side still shakes
}

// duration tells how many frames the note of channel i plays: until its
// envelope fades out or its length runs out, at most maxHold.
func (d *rumbleDetector) duration(i int) float64 {
	c := &d.g.APU.ch[i]
	f := d.maxHold()
	if c.env.period == 0 && !c.lengthOn {
		f = math.Min(f, d.params().Steady*d.params().Duration) // see RumbleParams.Steady
	}
	if c.env.period != 0 && !c.env.up {
		// One volume step every period/64 s.
		f = math.Min(f, float64(c.env.volume)*float64(c.env.period)*60/64)
	}
	if c.lengthOn {
		f = math.Min(f, float64(c.length)*60/256)
	}
	return math.Max(f, 1)
}

// lower sees a write to NR43 while the noise plays: an explosion often
// lowers its noise as it goes on, which makes its pulse stronger and
// longer, a few times at most. Games that rewrite the same noise, or raise
// it, again and again (R-Type DX) get nothing.
func (d *rumbleDetector) lower() {
	p := &d.pulses[3]
	if !p.active || !d.g.APU.ch[3].enabled {
		return
	}
	l := noiseLowness(d.g.APU.regs[0x12])
	if l > 0.5 && l > p.lowness+0.05 && p.lowered < maxLowered {
		p.strength = math.Min(1, p.strength+0.1*l)
		p.hold = math.Min(d.maxHold(), p.hold+4)
		p.lowered++
	}
	p.lowness = l
}

// frame works out how hard the frame just run shakes: the strongest pulse,
// fading with the envelope of its channel, or the screen shaking.
func (d *rumbleDetector) frame() {
	t := d.now()
	d.level = d.watchScreen(t)
	for i := range d.pulses {
		p := &d.pulses[i]
		if !p.active {
			continue
		}
		age := t - p.start
		c := &d.g.APU.ch[i]
		var v float64
		switch {
		case age > math.Max(p.hold, d.minHold()):
			p.active = false
			continue
		case age < d.minHold():
			v = p.strength // felt at full strength first
		case !c.enabled:
			p.active = false
			continue
		case p.volume > 0:
			v = p.strength * float64(c.env.volume) / float64(p.volume)
		}
		d.level = math.Max(d.level, v)
	}
}
