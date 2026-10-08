package gb

// Flag bits of the F register.
const (
	flagZ byte = 0x80
	flagN byte = 0x40
	flagH byte = 0x20
	flagC byte = 0x10
)

// CPU emulates the Sharp SM83 core. Every memory access and internal delay
// ticks the bus by one M-cycle, so the rest of the system stays in sync.
type CPU struct {
	a, f, b, c, d, e, h, l byte
	sp, pc                 uint16

	ime       bool
	eiPending bool
	halted    bool
	haltBug   bool
	locked    bool // an illegal opcode hangs the CPU

	bus *Bus
}

func (c *CPU) af() uint16 { return uint16(c.a)<<8 | uint16(c.f) }
func (c *CPU) bc() uint16 { return uint16(c.b)<<8 | uint16(c.c) }
func (c *CPU) de() uint16 { return uint16(c.d)<<8 | uint16(c.e) }
func (c *CPU) hl() uint16 { return uint16(c.h)<<8 | uint16(c.l) }

func (c *CPU) setAF(v uint16) { c.a, c.f = byte(v>>8), byte(v)&0xF0 }
func (c *CPU) setBC(v uint16) { c.b, c.c = byte(v>>8), byte(v) }
func (c *CPU) setDE(v uint16) { c.d, c.e = byte(v>>8), byte(v) }
func (c *CPU) setHL(v uint16) { c.h, c.l = byte(v>>8), byte(v) }

func (c *CPU) flag(f byte) bool { return c.f&f != 0 }

func (c *CPU) setFlags(z, n, h, cy bool) {
	var f byte
	if z {
		f |= flagZ
	}
	if n {
		f |= flagN
	}
	if h {
		f |= flagH
	}
	if cy {
		f |= flagC
	}
	c.f = f
}

func (c *CPU) tick() { c.bus.tick() }

func (c *CPU) read(addr uint16) byte {
	c.bus.tick()
	return c.bus.read(addr)
}

func (c *CPU) write(addr uint16, v byte) {
	c.bus.tick()
	c.bus.write(addr, v)
}

func (c *CPU) fetch() byte {
	v := c.read(c.pc)
	if c.haltBug {
		c.haltBug = false
	} else {
		c.pc++
	}
	return v
}

func (c *CPU) fetch16() uint16 {
	lo := c.fetch()
	return uint16(c.fetch())<<8 | uint16(lo)
}

func (c *CPU) push(v uint16) {
	c.tick()
	c.sp--
	c.write(c.sp, byte(v>>8))
	c.sp--
	c.write(c.sp, byte(v))
}

func (c *CPU) pop() uint16 {
	lo := c.read(c.sp)
	c.sp++
	hi := c.read(c.sp)
	c.sp++
	return uint16(hi)<<8 | uint16(lo)
}

// Step executes one instruction (or services an interrupt, or idles one
// M-cycle while halted).
func (c *CPU) Step() {
	if c.bus.stall > 0 { // VRAM DMA in progress
		c.bus.stall--
		c.tick()
		return
	}
	if c.locked {
		c.tick()
		return
	}
	pending := c.bus.pendingInterrupts()
	if pending != 0 {
		c.halted = false
		if c.ime {
			c.serviceInterrupt(pending)
			return
		}
	}
	if c.halted {
		if c.bus.frameEnd != 0 {
			if n := c.bus.quietTicks(); n > 0 {
				c.bus.skipQuiet(n)
			}
		}
		c.tick()
		return
	}
	enableIME := c.eiPending
	c.execute(c.fetch())
	if enableIME && c.eiPending {
		c.eiPending = false
		c.ime = true
	}
}

func (c *CPU) serviceInterrupt(pending byte) {
	c.ime = false
	c.tick()
	c.tick()
	c.sp--
	c.write(c.sp, byte(c.pc>>8))
	c.sp--
	c.write(c.sp, byte(c.pc))
	for i := range 5 {
		bit := byte(1) << i
		if pending&bit != 0 {
			c.bus.ifl &^= bit
			c.pc = 0x40 + uint16(i)*8
			break
		}
	}
	c.tick()
}

// getR/setR use the standard r[] encoding: B C D E H L (HL) A.
func (c *CPU) getR(i byte) byte {
	switch i {
	case 0:
		return c.b
	case 1:
		return c.c
	case 2:
		return c.d
	case 3:
		return c.e
	case 4:
		return c.h
	case 5:
		return c.l
	case 6:
		return c.read(c.hl())
	}
	return c.a
}

func (c *CPU) setR(i, v byte) {
	switch i {
	case 0:
		c.b = v
	case 1:
		c.c = v
	case 2:
		c.d = v
	case 3:
		c.e = v
	case 4:
		c.h = v
	case 5:
		c.l = v
	case 6:
		c.write(c.hl(), v)
	default:
		c.a = v
	}
}

// getRP/setRP use rp[]: BC DE HL SP.
func (c *CPU) getRP(p byte) uint16 {
	switch p {
	case 0:
		return c.bc()
	case 1:
		return c.de()
	case 2:
		return c.hl()
	}
	return c.sp
}

func (c *CPU) setRP(p byte, v uint16) {
	switch p {
	case 0:
		c.setBC(v)
	case 1:
		c.setDE(v)
	case 2:
		c.setHL(v)
	default:
		c.sp = v
	}
}

// cond evaluates cc[]: NZ Z NC C.
func (c *CPU) cond(y byte) bool {
	switch y & 3 {
	case 0:
		return !c.flag(flagZ)
	case 1:
		return c.flag(flagZ)
	case 2:
		return !c.flag(flagC)
	}
	return c.flag(flagC)
}

func (c *CPU) alu(op, v byte) {
	a := c.a
	switch op {
	case 0: // ADD
		r := uint16(a) + uint16(v)
		c.a = byte(r)
		c.setFlags(c.a == 0, false, a&0xF+v&0xF > 0xF, r > 0xFF)
	case 1: // ADC
		var cy byte
		if c.flag(flagC) {
			cy = 1
		}
		r := uint16(a) + uint16(v) + uint16(cy)
		c.a = byte(r)
		c.setFlags(c.a == 0, false, a&0xF+v&0xF+cy > 0xF, r > 0xFF)
	case 2: // SUB
		c.a = a - v
		c.setFlags(c.a == 0, true, a&0xF < v&0xF, a < v)
	case 3: // SBC
		var cy byte
		if c.flag(flagC) {
			cy = 1
		}
		r := int(a) - int(v) - int(cy)
		c.a = byte(r)
		c.setFlags(c.a == 0, true, int(a&0xF)-int(v&0xF)-int(cy) < 0, r < 0)
	case 4: // AND
		c.a = a & v
		c.setFlags(c.a == 0, false, true, false)
	case 5: // XOR
		c.a = a ^ v
		c.setFlags(c.a == 0, false, false, false)
	case 6: // OR
		c.a = a | v
		c.setFlags(c.a == 0, false, false, false)
	case 7: // CP
		c.setFlags(a == v, true, a&0xF < v&0xF, a < v)
	}
}

func (c *CPU) daa() {
	a := c.a
	var corr byte
	carry := c.flag(flagC)
	n := c.flag(flagN)
	if c.flag(flagH) || (!n && a&0xF > 9) {
		corr |= 0x06
	}
	if carry || (!n && a > 0x99) {
		corr |= 0x60
		carry = true
	}
	if n {
		a -= corr
	} else {
		a += corr
	}
	c.a = a
	c.setFlags(a == 0, n, false, carry)
}

// addSPe computes SP+e8 with the flags of ADD SP,e and LD HL,SP+e.
func (c *CPU) addSPe(e byte) uint16 {
	sp := c.sp
	r := uint16(int32(sp) + int32(int8(e)))
	c.setFlags(false, false, sp&0xF+uint16(e)&0xF > 0xF, sp&0xFF+uint16(e) > 0xFF)
	return r
}

func (c *CPU) execute(op byte) {
	x, y, z := op>>6, (op>>3)&7, op&7
	p, q := y>>1, y&1

	switch x {
	case 0:
		switch z {
		case 0:
			switch y {
			case 0: // NOP
			case 1: // LD (a16),SP
				addr := c.fetch16()
				c.write(addr, byte(c.sp))
				c.write(addr+1, byte(c.sp>>8))
			case 2: // STOP
				c.fetch()
				c.bus.timer.resetDiv()
				if c.bus.switchSpeed() {
					// The CPU is stopped while the clock settles.
					c.bus.stall += 2050
				}
			case 3: // JR e
				e := int8(c.fetch())
				c.tick()
				c.pc = uint16(int32(c.pc) + int32(e))
			default: // JR cc,e
				e := int8(c.fetch())
				if c.cond(y - 4) {
					c.tick()
					c.pc = uint16(int32(c.pc) + int32(e))
				}
			}
		case 1:
			if q == 0 { // LD rr,d16
				c.setRP(p, c.fetch16())
			} else { // ADD HL,rr
				hl, v := c.hl(), c.getRP(p)
				r := uint32(hl) + uint32(v)
				c.tick()
				c.f = c.f&flagZ | boolFlag(hl&0xFFF+v&0xFFF > 0xFFF, flagH) | boolFlag(r > 0xFFFF, flagC)
				c.setHL(uint16(r))
			}
		case 2:
			var addr uint16
			switch p {
			case 0:
				addr = c.bc()
			case 1:
				addr = c.de()
			case 2:
				addr = c.hl()
				c.setHL(addr + 1)
			case 3:
				addr = c.hl()
				c.setHL(addr - 1)
			}
			if q == 0 {
				c.write(addr, c.a)
			} else {
				c.a = c.read(addr)
			}
		case 3:
			c.tick()
			if q == 0 {
				c.setRP(p, c.getRP(p)+1)
			} else {
				c.setRP(p, c.getRP(p)-1)
			}
		case 4: // INC r
			v := c.getR(y)
			r := v + 1
			c.setR(y, r)
			c.f = c.f&flagC | boolFlag(r == 0, flagZ) | boolFlag(v&0xF == 0xF, flagH)
		case 5: // DEC r
			v := c.getR(y)
			r := v - 1
			c.setR(y, r)
			c.f = c.f&flagC | boolFlag(r == 0, flagZ) | flagN | boolFlag(v&0xF == 0, flagH)
		case 6: // LD r,d8
			v := c.fetch()
			c.setR(y, v)
		case 7:
			switch y {
			case 0: // RLCA
				c.a = c.rotate(0, c.a)
				c.f &^= flagZ
			case 1: // RRCA
				c.a = c.rotate(1, c.a)
				c.f &^= flagZ
			case 2: // RLA
				c.a = c.rotate(2, c.a)
				c.f &^= flagZ
			case 3: // RRA
				c.a = c.rotate(3, c.a)
				c.f &^= flagZ
			case 4:
				c.daa()
			case 5: // CPL
				c.a = ^c.a
				c.f |= flagN | flagH
			case 6: // SCF
				c.f = c.f&flagZ | flagC
			case 7: // CCF
				c.f = c.f&(flagZ|flagC) ^ flagC
			}
		}
	case 1:
		if op == 0x76 { // HALT
			if !c.ime && c.bus.pendingInterrupts() != 0 {
				c.haltBug = true
			} else {
				c.halted = true
			}
			return
		}
		c.setR(y, c.getR(z))
	case 2:
		c.alu(y, c.getR(z))
	case 3:
		c.execute3(op, y, z, p, q)
	}
}

func (c *CPU) execute3(op, y, z, p, q byte) {
	switch z {
	case 0:
		switch y {
		case 4: // LDH (a8),A
			c.write(0xFF00|uint16(c.fetch()), c.a)
		case 5: // ADD SP,e
			e := c.fetch()
			c.sp = c.addSPe(e)
			c.tick()
			c.tick()
		case 6: // LDH A,(a8)
			c.a = c.read(0xFF00 | uint16(c.fetch()))
		case 7: // LD HL,SP+e
			e := c.fetch()
			c.setHL(c.addSPe(e))
			c.tick()
		default: // RET cc
			c.tick()
			if c.cond(y) {
				c.pc = c.pop()
				c.tick()
			}
		}
	case 1:
		if q == 0 { // POP rr
			v := c.pop()
			switch p {
			case 0:
				c.setBC(v)
			case 1:
				c.setDE(v)
			case 2:
				c.setHL(v)
			case 3:
				c.setAF(v)
			}
			return
		}
		switch p {
		case 0: // RET
			c.pc = c.pop()
			c.tick()
		case 1: // RETI
			c.pc = c.pop()
			c.tick()
			c.ime = true
		case 2: // JP HL
			c.pc = c.hl()
		case 3: // LD SP,HL
			c.tick()
			c.sp = c.hl()
		}
	case 2:
		switch y {
		case 4: // LD (C),A
			c.write(0xFF00|uint16(c.c), c.a)
		case 5: // LD (a16),A
			c.write(c.fetch16(), c.a)
		case 6: // LD A,(C)
			c.a = c.read(0xFF00 | uint16(c.c))
		case 7: // LD A,(a16)
			c.a = c.read(c.fetch16())
		default: // JP cc,a16
			addr := c.fetch16()
			if c.cond(y) {
				c.tick()
				c.pc = addr
			}
		}
	case 3:
		switch y {
		case 0: // JP a16
			addr := c.fetch16()
			c.tick()
			c.pc = addr
		case 1:
			c.executeCB(c.fetch())
		case 6: // DI
			c.ime = false
			c.eiPending = false
		case 7: // EI
			c.eiPending = true
		default:
			c.locked = true
		}
	case 4:
		if y > 3 {
			c.locked = true
			return
		}
		addr := c.fetch16() // CALL cc,a16
		if c.cond(y) {
			c.push(c.pc)
			c.pc = addr
		}
	case 5:
		if q == 0 { // PUSH rr
			var v uint16
			switch p {
			case 0:
				v = c.bc()
			case 1:
				v = c.de()
			case 2:
				v = c.hl()
			case 3:
				v = c.af()
			}
			c.push(v)
			return
		}
		if p != 0 {
			c.locked = true
			return
		}
		addr := c.fetch16() // CALL a16
		c.push(c.pc)
		c.pc = addr
	case 6:
		c.alu(y, c.fetch())
	case 7: // RST
		c.push(c.pc)
		c.pc = uint16(y) * 8
	}
}

// rotate implements the CB rotation/shift group: RLC RRC RL RR SLA SRA SWAP SRL.
func (c *CPU) rotate(op, v byte) byte {
	var r byte
	var cy bool
	oldC := c.flag(flagC)
	switch op {
	case 0:
		cy = v&0x80 != 0
		r = v<<1 | v>>7
	case 1:
		cy = v&1 != 0
		r = v>>1 | v<<7
	case 2:
		cy = v&0x80 != 0
		r = v << 1
		if oldC {
			r |= 1
		}
	case 3:
		cy = v&1 != 0
		r = v >> 1
		if oldC {
			r |= 0x80
		}
	case 4:
		cy = v&0x80 != 0
		r = v << 1
	case 5:
		cy = v&1 != 0
		r = v>>1 | v&0x80
	case 6:
		r = v<<4 | v>>4
	case 7:
		cy = v&1 != 0
		r = v >> 1
	}
	c.setFlags(r == 0, false, false, cy)
	return r
}

func (c *CPU) executeCB(op byte) {
	x, y, z := op>>6, (op>>3)&7, op&7
	v := c.getR(z)
	switch x {
	case 0:
		c.setR(z, c.rotate(y, v))
	case 1: // BIT
		c.f = c.f&flagC | flagH | boolFlag(v&(1<<y) == 0, flagZ)
	case 2: // RES
		c.setR(z, v&^(1<<y))
	case 3: // SET
		c.setR(z, v|1<<y)
	}
}

func boolFlag(b bool, f byte) byte {
	if b {
		return f
	}
	return 0
}
