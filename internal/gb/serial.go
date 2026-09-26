package gb

// Serial implements the link port with nothing connected: transfers driven
// by the internal clock complete after 8 bits and shift in 0xFF. Outgoing
// bytes are recorded, which is how test ROMs report their results.
type Serial struct {
	bus       *Bus
	sb, sc    byte
	remaining int // T-cycles until the current transfer completes
	Output    []byte
}

func (s *Serial) read(addr uint16) byte {
	if addr == 0xFF01 {
		return s.sb
	}
	return s.sc | 0x7E
}

func (s *Serial) write(addr uint16, v byte) {
	if addr == 0xFF01 {
		s.sb = v
		return
	}
	s.sc = v
	if v&0x81 == 0x81 {
		s.Output = append(s.Output, s.sb)
		s.remaining = 8 * 512 // 8192 Hz clock
	}
}

func (s *Serial) tick() {
	if s.remaining <= 0 {
		return
	}
	s.remaining -= 4
	if s.remaining <= 0 {
		s.sb = 0xFF
		s.sc &^= 0x80
		s.bus.requestInterrupt(IntSerial)
	}
}
