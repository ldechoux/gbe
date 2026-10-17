package gb

import (
	"io"
	"sync"
	"time"
)

// The link cable joins the serial ports of two Game Boys. On the hardware,
// the one using its internal clock (the master) shifts its byte out to the
// other one (the slave) while shifting the slave's byte in, 8 bits in step.
// Over a network, a transfer is a message from the master, carrying its
// byte, answered by a message from the other side carrying the byte the
// master shifts in:
//
//   - a slave armed for a transfer (SC bit 7 set, external clock) answers
//     with SB, takes the master's byte and requests the serial interrupt;
//   - a Game Boy that is not armed shifts nothing and answers 0x00;
//   - one using its internal clock too answers 0xFF: when both start a
//     transfer at once, each shifts in 0xFF (as in SameBoy).
//
// The two emulators run each at their own pace, so the master waits for the
// answer when its transfer ends (at most linkTimeout). A slave is answered
// as of when its game has left SB and SC alone for linkSettle T-cycles:
// games often arm the port and then write the byte to send (Dr. Mario arms
// with 0x66, then writes the echo of the last byte), which on the hardware
// they have time to do before the master starts the next transfer. Once
// settled, it is answered at once by the goroutine reading the connection,
// even between two frames, so the master hardly waits. Before that, the
// transfer is held, until the port settles or, if it is not armed, for
// linkWindow T-cycles in case the game is about to arm it.

// Messages are 3 bytes: the kind, a sequence number and the data. The
// answer has the sequence number of the transfer, so that a late answer to
// a transfer that timed out is not taken for the answer to the next one.
const (
	msgTransfer = 1 // from the master: its byte
	msgReply    = 2 // to the master: the byte it shifts in
	msgComplete = 3 // not sent: a transfer the reader answered as a slave
)

// linkTimeout is how long, in real time, the master waits for an answer.
// After a timeout it stops waiting, shifting in 0xFF, until the other side
// sends something again (its game may be paused in the menu).
const linkTimeout = 200 * time.Millisecond

// linkWindow is how many T-cycles a transfer reaching a Game Boy that is not
// armed is held: about 4 transfers at 8192 Hz.
const linkWindow = 4 * 8 * 512

// linkSettle is how many T-cycles a slave's game leaves SB and SC alone
// before its answer is taken: Dr. Mario writes SB about 180 T-cycles after
// arming the port, and its master waits about 5400 between two transfers.
const linkSettle = 2048

// Link is a link cable plugged into another Game Boy, over a connection
// that carries the messages (see internal/link). It may be moved from one
// GameBoy to another, but plugged into one at a time (see SetLink).
type Link struct {
	conn   io.ReadWriteCloser
	events chan linkEvent // from the reader to the emulation
	out    chan [3]byte   // to the writer
	done   chan struct{}  // closed when the connection ends
	quit   chan struct{}  // closed by Close
	once   sync.Once

	mu sync.Mutex // guards the fields below, shared with the reader
	// armed reports whether the serial port is armed as a slave, and
	// settled (see linkSettle): the reader then answers a transfer with sb,
	// disarming it.
	armed bool
	// master reports whether the serial port is shifting with its internal
	// clock: the reader then answers a transfer with 0xFF.
	master bool
	sb     byte
}

type linkEvent struct {
	kind, seq, data byte
	at              time.Time // when it was received, for msgTransfer
}

// NewLink plugs the link cable into a connection with another Game Boy,
// reading it until it ends: see Done. The connection must have been set up
// by both sides (see internal/link).
func NewLink(conn io.ReadWriteCloser) *Link {
	l := &Link{
		conn:   conn,
		events: make(chan linkEvent, 64),
		out:    make(chan [3]byte, 256),
		done:   make(chan struct{}),
		quit:   make(chan struct{}),
	}
	go l.read()
	go l.write()
	return l
}

// Done is closed when the connection ends, by Close or on the other side.
func (l *Link) Done() <-chan struct{} { return l.done }

// Close unplugs the cable: the connection ends on both sides.
func (l *Link) Close() error {
	l.once.Do(func() { close(l.quit) })
	return l.conn.Close()
}

// post hands an event to the emulation, unless the cable is unplugged
// first: nothing may be reading them any more.
func (l *Link) post(e linkEvent) bool {
	select {
	case l.events <- e:
		return true
	case <-l.quit:
		return false
	}
}

func (l *Link) read() {
	defer close(l.done)
	var m [3]byte
	for {
		if _, err := io.ReadFull(l.conn, m[:]); err != nil {
			l.conn.Close()
			return
		}
		kind, seq, data := m[0], m[1], m[2]
		switch kind {
		case msgTransfer:
			l.mu.Lock()
			armed, master, sb := l.armed, l.master, l.sb
			l.armed = false
			l.mu.Unlock()
			switch {
			case armed:
				l.send(msgReply, seq, sb)
				if !l.post(linkEvent{kind: msgComplete, data: data}) {
					return
				}
				continue
			case master:
				l.send(msgReply, seq, 0xFF)
				continue
			}
		case msgReply:
		default: // not a gbe link: hang up
			l.conn.Close()
			return
		}
		if !l.post(linkEvent{kind: kind, seq: seq, data: data, at: time.Now()}) {
			return
		}
	}
}

// send queues a message for the writer, without ever waiting for the
// network: when the other side stops reading (its emulation paused), the
// queue fills up and messages are dropped. Its game times out instead.
func (l *Link) send(kind, seq, data byte) {
	select {
	case l.out <- [3]byte{kind, seq, data}:
	default:
	}
}

// write writes the queued messages. A failure ends the connection, which
// the reader then sees.
func (l *Link) write() {
	for {
		select {
		case m := <-l.out:
			if _, err := l.conn.Write(m[:]); err != nil {
				l.conn.Close()
				return
			}
		case <-l.done:
			return
		case <-l.quit:
			return
		}
	}
}

// setPort tells the reader how the serial port is set: armed as a slave,
// or shifting as a master.
func (l *Link) setPort(sc, sb byte) {
	l.mu.Lock()
	l.armed = false // until settled
	l.master = sc&0x81 == 0x81
	l.sb = sb
	l.mu.Unlock()
}

func (l *Link) setSB(sb byte) {
	l.mu.Lock()
	l.armed = false // until settled
	l.sb = sb
	l.mu.Unlock()
}

// setArmed tells the reader that the port armed as a slave has settled.
func (l *Link) setArmed() {
	l.mu.Lock()
	l.armed = true
	l.mu.Unlock()
}

// SetLink plugs the link cable into this Game Boy, or unplugs it (nil). It
// stays plugged in across Reset. While it is plugged in, the frames are run
// by RunLinkedFrame: RunFrame leaves the cable aside, as if unplugged.
func (g *GameBoy) SetLink(l *Link) {
	g.link = l
	g.Serial.plug(l)
}

// Link returns the link cable plugged in, nil without one.
func (g *GameBoy) Link() *Link { return g.link }

// RunLinkedFrame is RunFrame with a link cable plugged in (see SetLink):
// before each instruction, it hands the cable what the game wrote to the
// serial port and handles what came in. The halted CPU does not skip
// M-cycles, as a byte may come in at any time. The emulation is no longer
// deterministic.
//
// Being apart from RunFrame keeps the code run without a cable as it was:
// the emulation loop is sensitive to where its code lies in memory.
func (g *GameBoy) RunLinkedFrame() {
	if g.link == nil {
		g.RunFrame()
		return
	}
	start := g.Bus.cycles
	g.PPU.frameReady = false
	for !g.PPU.frameReady && g.Bus.cycles-start < CyclesPerFrame {
		g.Serial.poll()
		g.CPU.Step()
	}
	if g.guess != nil {
		g.guess.frame()
	}
	if g.Cart.rumble {
		g.rumble = g.Cart.motorShare(start, g.Bus.cycles)
	}
}

// plug connects the serial port to a link cable, nil to unplug it. A
// transfer in progress then ends as with nothing connected.
func (s *Serial) plug(l *Link) {
	if s.link != nil && s.waiting {
		s.remaining = 4 // ends with the next M-cycle
	}
	s.link, s.waiting, s.held, s.stalled, s.settled = l, false, false, false, false
	s.seenSB, s.seenSC, s.writtenAt = s.sb, s.sc, s.bus.cycles
	if l != nil {
		l.setPort(s.sc, s.sb)
	}
}

// written hands the cable what the game wrote to SB and SC since the last
// instruction. Serial.write is the same as with nothing connected: it
// starts a transfer with the internal clock by setting remaining, which is
// turned into linkEnd here.
func (s *Serial) written() {
	if s.remaining > 0 {
		s.linkEnd = s.bus.cycles + uint64(s.remaining/4*s.bus.dots())
		s.remaining = 0
		s.Output = s.Output[:0] // only recorded with nothing connected
		s.seenSB, s.seenSC = s.sb, s.sc
		s.startLinked()
		return
	}
	scChanged := s.sc != s.seenSC
	s.seenSB, s.seenSC = s.sb, s.sc
	s.writtenAt, s.settled = s.bus.cycles, false
	if scChanged {
		s.link.setPort(s.sc, s.sb)
	} else {
		s.link.setSB(s.sb)
	}
}

// startLinked begins a transfer as master over the link.
func (s *Serial) startLinked() {
	s.seq++
	s.waiting, s.answered = true, false
	if s.held { // both started a transfer at once
		s.held = false
		s.link.send(msgReply, s.heldSeq, 0xFF)
	}
	s.link.setPort(s.sc, s.sb)
	s.link.send(msgTransfer, s.seq, s.sb)
}

// settle tells the reader, or the transfer held, that the port armed as a
// slave has settled: it has been left alone for linkSettle T-cycles.
func (s *Serial) settle() {
	s.settled = true
	if s.held {
		s.answerHeld()
		return
	}
	s.link.setArmed()
}

// answerHeld answers the transfer held, as the port is now.
func (s *Serial) answerHeld() {
	s.held = false
	if s.sc&0x81 != 0x80 { // not armed: nothing shifted
		s.link.send(msgReply, s.heldSeq, 0x00)
		return
	}
	s.link.send(msgReply, s.heldSeq, s.sb)
	s.complete(s.heldData)
}

// complete ends a transfer, shifting in b.
func (s *Serial) complete(b byte) {
	s.sb = b
	s.sc &^= 0x80
	s.bus.requestInterrupt(IntSerial)
	s.seenSB, s.seenSC, s.settled = s.sb, s.sc, false
	if s.link != nil {
		s.link.setPort(s.sc, s.sb)
	}
}

// poll handles what came in through the cable, and ends the transfer
// started as master once its time is up.
func (s *Serial) poll() {
	if s.remaining > 0 || s.sb != s.seenSB || s.sc != s.seenSC {
		s.written()
	}
	s.drain()
	switch {
	case !s.settled && s.sc&0x81 == 0x80 && s.bus.cycles-s.writtenAt >= linkSettle:
		s.settle()
	case s.held && s.bus.cycles-s.heldAt >= linkWindow:
		s.answerHeld() // not armed or not settled in time: as it is
	}
	if s.waiting && s.bus.cycles >= s.linkEnd {
		s.complete(s.awaitAnswer())
	}
}

// drain handles the events already received.
func (s *Serial) drain() {
	for len(s.link.events) > 0 {
		s.handle(<-s.link.events)
	}
}

func (s *Serial) handle(e linkEvent) {
	s.stalled = false
	switch e.kind {
	case msgComplete:
		s.complete(e.data)
	case msgReply:
		if s.waiting && !s.answered && e.seq == s.seq {
			s.answered, s.answer = true, e.data
		}
	case msgTransfer:
		// The port was neither settled nor shifting when it came in. It may
		// have changed since.
		switch {
		case time.Since(e.at) > linkTimeout: // the master gave up
		case s.sc&0x81 == 0x80 && s.settled:
			s.link.send(msgReply, e.seq, s.sb)
			s.complete(e.data)
		case s.sc&0x81 == 0x81:
			s.link.send(msgReply, e.seq, 0xFF)
		default:
			if s.held { // a second one: the first was never answered
				s.link.send(msgReply, s.heldSeq, 0x00)
			}
			s.held, s.heldSeq, s.heldData, s.heldAt = true, e.seq, e.data, s.bus.cycles
		}
	}
}

// awaitAnswer waits for the answer to the transfer the master just ended,
// returning the byte it shifts in.
func (s *Serial) awaitAnswer() byte {
	s.drain()
	if !s.answered && !s.stalled {
		timeout := time.NewTimer(linkTimeout)
		defer timeout.Stop()
	wait:
		for !s.answered {
			select {
			case e := <-s.link.events:
				s.handle(e)
			case <-s.link.done:
				break wait
			case <-timeout.C:
				s.stalled = true
				break wait
			}
		}
	}
	s.waiting = false
	if !s.answered {
		return 0xFF
	}
	return s.answer
}
