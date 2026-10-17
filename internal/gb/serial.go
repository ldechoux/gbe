package gb

// Serial implements the link port. With nothing connected, transfers driven
// by the internal clock complete after 8 bits and shift in 0xFF, and
// outgoing bytes are recorded, which is how test ROMs report their results.
// With a link cable plugged in, see link.go.
type Serial struct {
	bus       *Bus
	sb, sc    byte
	remaining int // T-cycles until the current transfer completes
	Output    []byte

	// The link cable, nil with nothing connected, and the state of the
	// transfers through it (see link.go).
	link     *Link
	seq      byte // of the last transfer started as master
	waiting  bool // a transfer started as master awaits its answer
	answered bool // the answer came in: answer
	answer   byte
	stalled  bool // the last answer timed out (see linkTimeout)
	// held reports a transfer from the other side that came in while the
	// port was not armed (see linkWindow).
	held              bool
	heldSeq, heldData byte
	heldAt            uint64 // the bus cycle it came in
	// linkEnd is the bus cycle at which the transfer started as master
	// ends. remaining stays 0 meanwhile: tick, run on every M-cycle, is
	// left for the transfers with nothing connected.
	linkEnd uint64
	// seenSB and seenSC are SB and SC as the cable last saw them (see
	// written).
	seenSB, seenSC byte
}

func (s *Serial) read(addr uint16) byte {
	if addr == 0xFF01 {
		return s.sb
	}
	if s.bus.cgbMode() {
		return s.sc | 0x7C // bit 1 selects the fast clock
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
		if v&0x02 != 0 && s.bus.cgbMode() {
			s.remaining = 8 * 16 // 262144 Hz clock
		}
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
