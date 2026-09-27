package gb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

type mbc interface {
	readROM(addr uint16) byte
	writeROM(addr uint16, v byte)
	readRAM(addr uint16) byte
	writeRAM(addr uint16, v byte)
	sync(c *codec) // save state
}

// Cartridge holds the ROM, the external RAM and the memory bank controller.
type Cartridge struct {
	Title   string
	Type    byte
	Battery bool

	rom []byte
	ram []byte
	mbc mbc
	rtc *rtc // MBC3 only, nil otherwise

	ramDirty bool
}

var ramSizes = map[byte]int{0: 0, 1: 0x800, 2: 0x2000, 3: 0x8000, 4: 0x20000, 5: 0x10000}

// NewCartridge parses the ROM header and sets up the matching MBC.
func NewCartridge(rom []byte) (*Cartridge, error) {
	if len(rom) < 0x150 {
		return nil, errors.New("rom too small")
	}
	c := &Cartridge{
		Title: strings.TrimRight(string(bytes.TrimRight(rom[0x134:0x144], "\x00")), " "),
		Type:  rom[0x147],
	}
	// Pad the ROM to a power of two so bank offsets can simply wrap.
	size := 0x8000
	for size < len(rom) {
		size <<= 1
	}
	c.rom = make([]byte, size)
	copy(c.rom, rom)
	for i := len(rom); i < size; i++ {
		c.rom[i] = 0xFF
	}
	ramSize := ramSizes[rom[0x149]]

	switch c.Type {
	case 0x00:
		c.mbc = &romOnly{c: c}
	case 0x08, 0x09:
		c.mbc = &romOnly{c: c}
		c.Battery = c.Type == 0x09
	case 0x01, 0x02, 0x03:
		c.mbc = &mbc1{c: c, bank1: 1}
		c.Battery = c.Type == 0x03
	case 0x05, 0x06:
		c.mbc = &mbc2{c: c, bank: 1}
		c.Battery = c.Type == 0x06
		ramSize = 512
	case 0x0F, 0x10, 0x11, 0x12, 0x13:
		c.rtc = &rtc{last: time.Now().Unix()}
		c.mbc = &mbc3{c: c, bank: 1}
		c.Battery = c.Type == 0x0F || c.Type == 0x10 || c.Type == 0x13
	case 0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E:
		c.mbc = &mbc5{c: c, bank: 1}
		c.Battery = c.Type == 0x1B || c.Type == 0x1E
	default:
		return nil, fmt.Errorf("unsupported cartridge type 0x%02X", c.Type)
	}
	c.ram = make([]byte, ramSize)
	return c, nil
}

func (c *Cartridge) readROM(addr uint16) byte     { return c.mbc.readROM(addr) }
func (c *Cartridge) writeROM(addr uint16, v byte) { c.mbc.writeROM(addr, v) }
func (c *Cartridge) readRAM(addr uint16) byte     { return c.mbc.readRAM(addr) }
func (c *Cartridge) writeRAM(addr uint16, v byte) { c.mbc.writeRAM(addr, v) }

func (c *Cartridge) romByte(bank int, addr uint16) byte {
	return c.rom[(bank*0x4000+int(addr&0x3FFF))&(len(c.rom)-1)]
}

func (c *Cartridge) ramOffset(bank int, addr uint16) int {
	return (bank*0x2000 + int(addr&0x1FFF)) % len(c.ram)
}

// Dirty reports whether battery-backed state changed since the last save.
func (c *Cartridge) Dirty() bool { return c.Battery && c.ramDirty }

// SaveData serializes the battery-backed RAM, followed for MBC3 by the RTC
// state in the 48-byte format used by BGB and VBA-M.
func (c *Cartridge) SaveData() []byte {
	c.ramDirty = false
	out := append([]byte(nil), c.ram...)
	if c.rtc != nil {
		out = append(out, c.rtc.marshal()...)
	}
	return out
}

// LoadSaveData restores what SaveData produced.
func (c *Cartridge) LoadSaveData(data []byte) {
	n := copy(c.ram, data)
	if c.rtc != nil && len(data) >= n+48 {
		c.rtc.unmarshal(data[n : n+48])
	}
}

type romOnly struct{ c *Cartridge }

func (m *romOnly) readROM(addr uint16) byte {
	return m.c.rom[int(addr)&(len(m.c.rom)-1)]
}
func (m *romOnly) writeROM(uint16, byte) {}
func (m *romOnly) readRAM(addr uint16) byte {
	if len(m.c.ram) == 0 {
		return 0xFF
	}
	return m.c.ram[m.c.ramOffset(0, addr)]
}
func (m *romOnly) writeRAM(addr uint16, v byte) {
	if len(m.c.ram) == 0 {
		return
	}
	m.c.ram[m.c.ramOffset(0, addr)] = v
	m.c.ramDirty = true
}

type mbc1 struct {
	c         *Cartridge
	ramEnable bool
	bank1     byte // 5 bits
	bank2     byte // 2 bits
	mode      byte
}

func (m *mbc1) readROM(addr uint16) byte {
	if addr < 0x4000 {
		bank := 0
		if m.mode == 1 {
			bank = int(m.bank2) << 5
		}
		return m.c.romByte(bank, addr)
	}
	return m.c.romByte(int(m.bank2)<<5|int(m.bank1), addr)
}

func (m *mbc1) writeROM(addr uint16, v byte) {
	switch {
	case addr < 0x2000:
		m.ramEnable = v&0x0F == 0x0A
	case addr < 0x4000:
		m.bank1 = v & 0x1F
		if m.bank1 == 0 {
			m.bank1 = 1
		}
	case addr < 0x6000:
		m.bank2 = v & 0x03
	default:
		m.mode = v & 1
	}
}

func (m *mbc1) ramBank() int {
	if m.mode == 1 {
		return int(m.bank2)
	}
	return 0
}

func (m *mbc1) readRAM(addr uint16) byte {
	if !m.ramEnable || len(m.c.ram) == 0 {
		return 0xFF
	}
	return m.c.ram[m.c.ramOffset(m.ramBank(), addr)]
}

func (m *mbc1) writeRAM(addr uint16, v byte) {
	if !m.ramEnable || len(m.c.ram) == 0 {
		return
	}
	m.c.ram[m.c.ramOffset(m.ramBank(), addr)] = v
	m.c.ramDirty = true
}

type mbc2 struct {
	c         *Cartridge
	ramEnable bool
	bank      byte
}

func (m *mbc2) readROM(addr uint16) byte {
	if addr < 0x4000 {
		return m.c.romByte(0, addr)
	}
	return m.c.romByte(int(m.bank), addr)
}

func (m *mbc2) writeROM(addr uint16, v byte) {
	if addr >= 0x4000 {
		return
	}
	if addr&0x100 == 0 {
		m.ramEnable = v&0x0F == 0x0A
	} else {
		m.bank = v & 0x0F
		if m.bank == 0 {
			m.bank = 1
		}
	}
}

func (m *mbc2) readRAM(addr uint16) byte {
	if !m.ramEnable {
		return 0xFF
	}
	return m.c.ram[addr&0x1FF] | 0xF0
}

func (m *mbc2) writeRAM(addr uint16, v byte) {
	if !m.ramEnable {
		return
	}
	m.c.ram[addr&0x1FF] = v & 0x0F
	m.c.ramDirty = true
}

type mbc3 struct {
	c         *Cartridge
	ramEnable bool
	bank      byte
	ramSel    byte // 0-3 RAM bank, 0x08-0x0C RTC register
	latchPrev byte
}

func (m *mbc3) readROM(addr uint16) byte {
	if addr < 0x4000 {
		return m.c.romByte(0, addr)
	}
	return m.c.romByte(int(m.bank), addr)
}

func (m *mbc3) writeROM(addr uint16, v byte) {
	switch {
	case addr < 0x2000:
		m.ramEnable = v&0x0F == 0x0A
	case addr < 0x4000:
		m.bank = v & 0x7F
		if m.bank == 0 {
			m.bank = 1
		}
	case addr < 0x6000:
		m.ramSel = v
	default:
		if m.latchPrev == 0 && v == 1 && m.c.rtc != nil {
			m.c.rtc.latch(time.Now().Unix())
		}
		m.latchPrev = v
	}
}

func (m *mbc3) readRAM(addr uint16) byte {
	if !m.ramEnable {
		return 0xFF
	}
	if m.ramSel >= 0x08 && m.ramSel <= 0x0C {
		if m.c.rtc == nil {
			return 0xFF
		}
		return m.c.rtc.latched[m.ramSel-0x08]
	}
	if len(m.c.ram) == 0 || m.ramSel > 3 {
		return 0xFF
	}
	return m.c.ram[m.c.ramOffset(int(m.ramSel), addr)]
}

func (m *mbc3) writeRAM(addr uint16, v byte) {
	if !m.ramEnable {
		return
	}
	if m.ramSel >= 0x08 && m.ramSel <= 0x0C {
		if m.c.rtc != nil {
			m.c.rtc.set(time.Now().Unix(), int(m.ramSel-0x08), v)
			m.c.ramDirty = true
		}
		return
	}
	if len(m.c.ram) == 0 || m.ramSel > 3 {
		return
	}
	m.c.ram[m.c.ramOffset(int(m.ramSel), addr)] = v
	m.c.ramDirty = true
}

type mbc5 struct {
	c         *Cartridge
	ramEnable bool
	bank      int // 9 bits
	ramBank   int
}

func (m *mbc5) readROM(addr uint16) byte {
	if addr < 0x4000 {
		return m.c.romByte(0, addr)
	}
	return m.c.romByte(m.bank, addr)
}

func (m *mbc5) writeROM(addr uint16, v byte) {
	switch {
	case addr < 0x2000:
		m.ramEnable = v&0x0F == 0x0A
	case addr < 0x3000:
		m.bank = m.bank&0x100 | int(v)
	case addr < 0x4000:
		m.bank = m.bank&0xFF | int(v&1)<<8
	case addr < 0x6000:
		m.ramBank = int(v & 0x0F)
	}
}

func (m *mbc5) readRAM(addr uint16) byte {
	if !m.ramEnable || len(m.c.ram) == 0 {
		return 0xFF
	}
	return m.c.ram[m.c.ramOffset(m.ramBank, addr)]
}

func (m *mbc5) writeRAM(addr uint16, v byte) {
	if !m.ramEnable || len(m.c.ram) == 0 {
		return
	}
	m.c.ram[m.c.ramOffset(m.ramBank, addr)] = v
	m.c.ramDirty = true
}

// rtc is the MBC3 real time clock, advanced from the host wall clock.
type rtc struct {
	s, m, h byte
	d       uint16 // 9 bits
	halt    bool
	carry   bool
	latched [5]byte
	last    int64 // unix time of the last update
}

func (r *rtc) update(now int64) {
	delta := now - r.last
	r.last = now
	if r.halt || delta <= 0 {
		return
	}
	total := int64(r.s) + int64(r.m)*60 + int64(r.h)*3600 + int64(r.d)*86400 + delta
	r.s = byte(total % 60)
	r.m = byte(total / 60 % 60)
	r.h = byte(total / 3600 % 24)
	days := total / 86400
	if days > 511 {
		r.carry = true
		days %= 512
	}
	r.d = uint16(days)
}

func (r *rtc) regs() [5]byte {
	dh := byte(r.d>>8) & 1
	if r.halt {
		dh |= 0x40
	}
	if r.carry {
		dh |= 0x80
	}
	return [5]byte{r.s, r.m, r.h, byte(r.d), dh}
}

func (r *rtc) latch(now int64) {
	r.update(now)
	r.latched = r.regs()
}

func (r *rtc) set(now int64, reg int, v byte) {
	r.update(now)
	switch reg {
	case 0:
		r.s = v % 60
	case 1:
		r.m = v % 60
	case 2:
		r.h = v % 24
	case 3:
		r.d = r.d&0x100 | uint16(v)
	case 4:
		r.d = r.d&0xFF | uint16(v&1)<<8
		r.halt = v&0x40 != 0
		r.carry = v&0x80 != 0
	}
	r.latched[reg] = r.regs()[reg]
}

func (r *rtc) marshal() []byte {
	r.update(time.Now().Unix())
	out := make([]byte, 48)
	cur := r.regs()
	for i := range 5 {
		binary.LittleEndian.PutUint32(out[i*4:], uint32(cur[i]))
		binary.LittleEndian.PutUint32(out[20+i*4:], uint32(r.latched[i]))
	}
	binary.LittleEndian.PutUint64(out[40:], uint64(r.last))
	return out
}

func (r *rtc) unmarshal(data []byte) {
	var cur [5]byte
	for i := range 5 {
		cur[i] = byte(binary.LittleEndian.Uint32(data[i*4:]))
		r.latched[i] = byte(binary.LittleEndian.Uint32(data[20+i*4:]))
	}
	r.s, r.m, r.h = cur[0]%60, cur[1]%60, cur[2]%24
	r.d = uint16(cur[3]) | uint16(cur[4]&1)<<8
	r.halt = cur[4]&0x40 != 0
	r.carry = cur[4]&0x80 != 0
	r.last = int64(binary.LittleEndian.Uint64(data[40:]))
	r.update(time.Now().Unix())
}
