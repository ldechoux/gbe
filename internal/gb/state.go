package gb

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// Save states are gzip-compressed. The payload starts with a magic, a format
// version and the identity of the ROM, followed by every component's state.
const (
	stateMagic   = "GBESTATE"
	stateVersion = 1
)

// ErrStateMismatch is returned when a save state belongs to another ROM.
var ErrStateMismatch = errors.New("save state was made with a different ROM")

// codec serializes state in both directions with the same code: each
// component lists its fields once in sync(), whether saving or loading.
type codec struct {
	w   *bytes.Buffer // saving
	r   io.Reader     // loading
	err error
}

func (c *codec) loading() bool { return c.r != nil }

func (c *codec) raw(b []byte) {
	if c.err != nil {
		return
	}
	if c.loading() {
		_, c.err = io.ReadFull(c.r, b)
	} else {
		c.w.Write(b)
	}
}

func (c *codec) u8(p *byte) {
	b := []byte{*p}
	c.raw(b)
	*p = b[0]
}

func (c *codec) u16(p *uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], *p)
	c.raw(b[:])
	*p = binary.LittleEndian.Uint16(b[:])
}

func (c *codec) u64(p *uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], *p)
	c.raw(b[:])
	*p = binary.LittleEndian.Uint64(b[:])
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

// romID identifies a ROM by its title and header/global checksums.
func (c *Cartridge) romID() []byte { return append([]byte(nil), c.rom[0x134:0x150]...) }

// SaveState captures the whole machine state.
func (g *GameBoy) SaveState() []byte {
	c := &codec{w: &bytes.Buffer{}}
	c.raw([]byte(stateMagic))
	version := uint16(stateVersion)
	c.u16(&version)
	c.raw(g.Cart.romID())
	g.sync(c)

	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	zw.Write(c.w.Bytes())
	zw.Close()
	return out.Bytes()
}

// LoadState restores a state made by SaveState. On error the machine is left
// untouched.
func (g *GameBoy) LoadState(data []byte) error {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("invalid save state: %w", err)
	}
	c := &codec{r: zr}
	magic := make([]byte, len(stateMagic))
	c.raw(magic)
	var version uint16
	c.u16(&version)
	id := make([]byte, len(g.Cart.romID()))
	c.raw(id)
	switch {
	case c.err != nil || string(magic) != stateMagic:
		return errors.New("invalid save state")
	case version != stateVersion:
		return fmt.Errorf("unsupported save state version %d", version)
	case !bytes.Equal(id, g.Cart.romID()):
		return ErrStateMismatch
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
	c.raw(b.wram[:])
	c.raw(b.hram[:])
	c.u8(&b.ie)
	c.u8(&b.ifl)
	c.bool(&b.bootEnabled)
	c.u64(&b.cycles)
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
	c.raw(p.vram[:])
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
}

func (a *APU) sync(c *codec) {
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
