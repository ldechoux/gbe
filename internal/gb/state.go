package gb

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
)

// Save states are gzip-compressed. The payload starts with a magic, a format
// version and the identity of the ROM, followed by every component's state.
const (
	stateMagic = "GBESTATE"
	// 2 added the Game Boy Color, 3 its compatibility mode; older states
	// still load.
	stateVersion = 3
)

// ErrStateMismatch is returned when a save state belongs to another ROM.
var ErrStateMismatch = errors.New("save state was made with a different ROM")

// codec serializes state in both directions with the same code: each
// component lists its fields once in sync(), whether saving or loading.
// It works on byte slices, so that the many snapshots taken for the rewind
// do not allocate.
type codec struct {
	out     []byte // saving: the data written so far
	in      []byte // loading: the data left to read
	load    bool
	version uint16 // format of the data being read or written
	err     error
}

func (c *codec) loading() bool { return c.load }

// next consumes the next n bytes of the data being loaded, or returns nil
// once it is exhausted.
func (c *codec) next(n int) []byte {
	if c.err != nil {
		return nil
	}
	if len(c.in) < n {
		c.err = io.ErrUnexpectedEOF
		return nil
	}
	b := c.in[:n]
	c.in = c.in[n:]
	return b
}

func (c *codec) raw(b []byte) {
	switch {
	case !c.load:
		c.out = append(c.out, b...)
	case c.err == nil:
		copy(b, c.next(len(b)))
	}
}

func (c *codec) u8(p *byte) {
	if !c.load {
		c.out = append(c.out, *p)
	} else if b := c.next(1); b != nil {
		*p = b[0]
	}
}

func (c *codec) u16(p *uint16) {
	if !c.load {
		c.out = binary.LittleEndian.AppendUint16(c.out, *p)
	} else if b := c.next(2); b != nil {
		*p = binary.LittleEndian.Uint16(b)
	}
}

// u16s is u16 for every element of s, in one go.
func (c *codec) u16s(s []uint16) {
	if !c.load {
		n := len(c.out)
		c.out = slices.Grow(c.out, 2*len(s))[:n+2*len(s)]
		b := c.out[n:]
		i := 0
		for ; i+4 <= len(s); i += 4 { // 4 at a time, much faster
			binary.LittleEndian.PutUint64(b[2*i:], uint64(s[i])|uint64(s[i+1])<<16|uint64(s[i+2])<<32|uint64(s[i+3])<<48)
		}
		for ; i < len(s); i++ {
			binary.LittleEndian.PutUint16(b[2*i:], s[i])
		}
	} else if b := c.next(2 * len(s)); b != nil {
		i := 0
		for ; i+4 <= len(s); i += 4 {
			v := binary.LittleEndian.Uint64(b[2*i:])
			s[i], s[i+1], s[i+2], s[i+3] = uint16(v), uint16(v>>16), uint16(v>>32), uint16(v>>48)
		}
		for ; i < len(s); i++ {
			s[i] = binary.LittleEndian.Uint16(b[2*i:])
		}
	}
}

func (c *codec) u64(p *uint64) {
	if !c.load {
		c.out = binary.LittleEndian.AppendUint64(c.out, *p)
	} else if b := c.next(8); b != nil {
		*p = binary.LittleEndian.Uint64(b)
	}
}

func (c *codec) i64(p *int64) {
	v := uint64(*p)
	c.u64(&v)
	*p = int64(v)
}

func (c *codec) int(p *int) {
	v := int64(*p)
	c.i64(&v)
	*p = int(v)
}

func (c *codec) bool(p *bool) {
	var v byte
	if *p {
		v = 1
	}
	c.u8(&v)
	*p = v != 0
}

func (c *codec) f64(p *float64) {
	v := math.Float64bits(*p)
	c.u64(&v)
	*p = math.Float64frombits(v)
}

// romIDSize is the length of romID.
const romIDSize = 0x150 - 0x134

// romID identifies a ROM by its title and header/global checksums.
func (c *Cartridge) romID() []byte { return append([]byte(nil), c.rom[0x134:0x150]...) }

// SaveState captures the whole machine state.
func (g *GameBoy) SaveState() []byte { return g.encodeState(stateVersion) }

// encodeState writes a save state in the given format version (older ones
// are only written by tests).
func (g *GameBoy) encodeState(version uint16) []byte {
	c := &codec{version: version}
	c.raw([]byte(stateMagic))
	c.u16(&version)
	c.raw(g.Cart.romID())
	if version >= 2 {
		model := byte(g.model)
		c.u8(&model)
	}
	g.sync(c)

	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	zw.Write(c.out)
	zw.Close()
	return out.Bytes()
}

// stateDecoder decompresses a save state, ready to be read.
func stateDecoder(data []byte) (*codec, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err == nil {
		data, err = io.ReadAll(zr)
	}
	if err != nil {
		return nil, fmt.Errorf("invalid save state: %w", err)
	}
	return &codec{in: data, load: true}, nil
}

// StateModel returns the model a state made by SaveState was made on.
func StateModel(data []byte) (Model, error) {
	c, err := stateDecoder(data)
	if err != nil {
		return 0, err
	}
	magic := make([]byte, len(stateMagic))
	c.raw(magic)
	var version uint16
	c.u16(&version)
	c.raw(make([]byte, romIDSize))
	model := ModelDMG
	if version >= 2 {
		var m byte
		c.u8(&m)
		model = Model(m)
	}
	if c.err != nil || string(magic) != stateMagic || version == 0 || version > stateVersion {
		return 0, errors.New("invalid save state")
	}
	return model, nil
}

// LoadState restores a state made by SaveState. On error the machine is left
// untouched.
func (g *GameBoy) LoadState(data []byte) error {
	c, err := stateDecoder(data)
	if err != nil {
		return err
	}
	magic := make([]byte, len(stateMagic))
	c.raw(magic)
	var version uint16
	c.u16(&version)
	id := make([]byte, len(g.Cart.romID()))
	c.raw(id)
	switch {
	case c.err != nil || string(magic) != stateMagic:
		return errors.New("invalid save state")
	case version == 0 || version > stateVersion:
		return fmt.Errorf("unsupported save state version %d", version)
	case !bytes.Equal(id, g.Cart.romID()):
		return ErrStateMismatch
	}
	c.version = version
	model := ModelDMG
	if version >= 2 {
		var m byte
		c.u8(&m)
		model = Model(m)
	}
	if c.err == nil && model != g.model {
		return errors.New("save state was made in another hardware mode (DMG / Game Boy Color)")
	}

	backup := g.SaveState()
	g.Reset()
	g.sync(c)
	if c.err == nil && g.Bus.bootEnabled && g.boot == nil {
		c.err = errors.New("save state was made during the boot ROM, which is not loaded")
	}
	if c.err != nil {
		g.LoadState(backup)
		return fmt.Errorf("invalid save state: %w", c.err)
	}
	return nil
}

// Snapshot captures the machine state for Restore, without the header and
// compression of SaveState: it is meant to be taken many times per second
// (rewind). It is written into buf, reused when large enough.
func (g *GameBoy) Snapshot(buf []byte) []byte {
	c := &codec{out: buf[:0], version: stateVersion}
	g.sync(c)
	return c.out
}

// Restore goes back to a state taken by Snapshot on this same machine. On
// error the machine is left in an undefined state.
func (g *GameBoy) Restore(data []byte) error {
	c := &codec{in: data, load: true, version: stateVersion}
	// Unlike LoadState, which may read older formats, there is no need to
	// Reset first: the snapshot overwrites the whole machine but for the
	// following, which are reset as Reset would. The sample rate, which
	// belongs to the frontend, is kept.
	g.sync(c)
	g.PPU.frameReady = false
	g.Joypad.pressed = [8]bool{}
	g.Serial.Output = nil
	g.APU.samples = g.APU.samples[:0]
	if c.err != nil {
		return fmt.Errorf("invalid snapshot: %w", c.err)
	}
	return nil
}

func (g *GameBoy) sync(c *codec) {
	g.CPU.sync(c)
	g.Bus.sync(c)
	g.Timer.sync(c)
	c.u8(&g.Joypad.sel)
	g.Serial.sync(c)
	g.PPU.sync(c)
	g.APU.sync(c)
	g.Cart.sync(c)
}

func (cpu *CPU) sync(c *codec) {
	for _, r := range []*byte{&cpu.a, &cpu.f, &cpu.b, &cpu.c, &cpu.d, &cpu.e, &cpu.h, &cpu.l} {
		c.u8(r)
	}
	c.u16(&cpu.sp)
	c.u16(&cpu.pc)
	for _, b := range []*bool{&cpu.ime, &cpu.eiPending, &cpu.halted, &cpu.haltBug, &cpu.locked} {
		c.bool(b)
	}
}

func (b *Bus) sync(c *codec) {
	if c.version < 2 {
		c.raw(b.wram[:0x2000])
	} else {
		c.raw(b.wram[:])
	}
	c.raw(b.hram[:])
	c.u8(&b.ie)
	c.u8(&b.ifl)
	c.bool(&b.bootEnabled)
	c.u64(&b.cycles)
	if c.version < 2 {
		return
	}
	c.u8(&b.svbk)
	c.bool(&b.doubleSpeed)
	c.bool(&b.speedArmed)
	c.u16(&b.hdmaSrc)
	c.u16(&b.hdmaDst)
	c.u8(&b.hdmaLen)
	c.bool(&b.hdmaActive)
	c.int(&b.stall)
	if c.version < 3 {
		return
	}
	c.u8(&b.key0)
	c.bool(&b.compat)
}

func (t *Timer) sync(c *codec) {
	c.u16(&t.counter)
	c.u8(&t.tima)
	c.u8(&t.tma)
	c.u8(&t.tac)
	c.bool(&t.overflow)
}

func (s *Serial) sync(c *codec) {
	c.u8(&s.sb)
	c.u8(&s.sc)
	c.int(&s.remaining)
}

func (p *PPU) sync(c *codec) {
	if c.version < 2 {
		c.raw(p.vram[:0x2000])
	} else {
		c.raw(p.vram[:])
	}
	c.raw(p.oam[:])
	for _, r := range []*byte{&p.lcdc, &p.stat, &p.scy, &p.scx, &p.ly, &p.lyc,
		&p.bgp, &p.obp0, &p.obp1, &p.wy, &p.wx, &p.dmaReg, &p.mode} {
		c.u8(r)
	}
	c.int(&p.dot)
	c.bool(&p.statLine)
	c.int(&p.windowLine)
	c.bool(&p.wyReached)
	c.raw(p.back[:])
	c.raw(p.front[:])
	if c.version < 2 {
		return
	}
	for _, r := range []*byte{&p.vbk, &p.bcps, &p.ocps, &p.opri} {
		c.u8(r)
	}
	c.raw(p.bgPal[:])
	c.raw(p.objPal[:])
	c.u16s(p.cback[:])
	c.u16s(p.cfront[:])
}

func (a *APU) sync(c *codec) {
	a.mixValid = false
	a.catchUp()
	a.untilClock = 0
	c.raw(a.regs[:])
	c.bool(&a.on)
	for i := range a.ch {
		a.ch[i].sync(c)
	}
	c.int(&a.seqTimer)
	c.int(&a.seqStep)
	c.f64(&a.sampleClock)
	c.f64(&a.accL)
	c.f64(&a.accR)
	c.int(&a.accN)
	c.f64(&a.hpL)
	c.f64(&a.hpR)
}

func (ch *channel) sync(c *codec) {
	c.bool(&ch.enabled)
	c.bool(&ch.dac)
	c.int(&ch.length)
	c.bool(&ch.lengthOn)
	c.int(&ch.freq)
	c.int(&ch.timer)
	c.u8(&ch.env.initial)
	c.bool(&ch.env.up)
	c.u8(&ch.env.period)
	c.u8(&ch.env.volume)
	c.u8(&ch.env.timer)
	c.u8(&ch.duty)
	c.u8(&ch.dutyPos)
	c.u8(&ch.wavePos)
	c.u8(&ch.waveShift)
	c.u16(&ch.lfsr)
	c.bool(&ch.narrow)
	c.u8(&ch.sweepPeriod)
	c.bool(&ch.sweepNegate)
	c.u8(&ch.sweepShift)
	c.u8(&ch.sweepTimer)
	c.bool(&ch.sweepOn)
	c.int(&ch.sweepShadow)
	c.bool(&ch.sweepNegUsed)
}

func (cart *Cartridge) sync(c *codec) {
	c.raw(cart.ram)
	cart.mbc.sync(c)
	if cart.rtc != nil {
		r := cart.rtc
		c.u8(&r.s)
		c.u8(&r.m)
		c.u8(&r.h)
		c.u16(&r.d)
		c.bool(&r.halt)
		c.bool(&r.carry)
		c.raw(r.latched[:])
		c.i64(&r.last)
	}
	if c.loading() {
		cart.ramDirty = true // the restored RAM must reach the .sav file
	}
}

func (m *romOnly) sync(*codec) {}

func (m *mbc1) sync(c *codec) {
	c.bool(&m.ramEnable)
	c.u8(&m.bank1)
	c.u8(&m.bank2)
	c.u8(&m.mode)
}

func (m *mbc2) sync(c *codec) {
	c.bool(&m.ramEnable)
	c.u8(&m.bank)
}

func (m *mbc3) sync(c *codec) {
	c.bool(&m.ramEnable)
	c.u8(&m.bank)
	c.u8(&m.ramSel)
	c.u8(&m.latchPrev)
}

func (m *mbc5) sync(c *codec) {
	c.bool(&m.ramEnable)
	c.int(&m.bank)
	c.int(&m.ramBank)
}
