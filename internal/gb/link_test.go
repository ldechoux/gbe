package gb

import (
	"bytes"
	"io"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// peer is the other end of a link cable, played by the test: it records the
// messages the Game Boy sends.
type peer struct {
	t    *testing.T
	conn net.Conn
	msgs chan [3]byte
}

// newLinkedGB returns a Game Boy with a link cable plugged into a peer.
func newLinkedGB(t *testing.T) (*GameBoy, *peer) {
	t.Helper()
	g := newTestGB(t)
	a, b := net.Pipe()
	p := &peer{t: t, conn: b, msgs: make(chan [3]byte, 16)}
	go func() {
		defer close(p.msgs)
		var m [3]byte
		for {
			if _, err := io.ReadFull(b, m[:]); err != nil {
				return
			}
			p.msgs <- m
		}
	}()
	l := NewLink(a)
	g.SetLink(l)
	t.Cleanup(func() { l.Close(); b.Close() })
	return g, p
}

func (p *peer) send(kind, seq, data byte) {
	p.t.Helper()
	if _, err := p.conn.Write([]byte{kind, seq, data}); err != nil {
		p.t.Fatal(err)
	}
}

// expect waits for the next message from the Game Boy.
func (p *peer) expect(kind, data byte) (seq byte) {
	p.t.Helper()
	select {
	case m, ok := <-p.msgs:
		if !ok {
			p.t.Fatal("connection closed")
		}
		if m[0] != kind || m[2] != data {
			p.t.Fatalf("got message %d %02X, want %d %02X", m[0], m[2], kind, data)
		}
		return m[1]
	case <-time.After(time.Second):
		p.t.Fatalf("no message, want %d %02X", kind, data)
	}
	return 0
}

func (p *peer) expectNone() {
	p.t.Helper()
	select {
	case m := <-p.msgs:
		p.t.Fatalf("unexpected message %d %02X", m[0], m[2])
	case <-time.After(20 * time.Millisecond):
	}
}

// pollUntil reads the cable until the serial interrupt is requested.
func pollUntil(t *testing.T, g *GameBoy) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for g.Bus.ifl&IntSerial == 0 {
		if time.Now().After(deadline) {
			t.Fatal("serial interrupt not requested")
		}
		g.Serial.poll()
		time.Sleep(time.Millisecond)
	}
}

// startMaster starts a transfer of b with the internal clock and checks the
// message sent.
func startMaster(t *testing.T, g *GameBoy, p *peer, b byte) (seq byte) {
	t.Helper()
	g.Bus.write(0xFF01, b)
	g.Bus.write(0xFF02, 0x81)
	g.Serial.poll()
	return p.expect(msgTransfer, b)
}

// step runs the serial port for an M-cycle, as RunFrame does.
func step(g *GameBoy) {
	g.Bus.cycles += 4
	if g.Serial.link != nil {
		g.Serial.poll()
	} else {
		g.Serial.tick()
	}
}

// finish runs the M-cycles of a transfer at 8192 Hz, checking that it does
// not end early.
func finish(t *testing.T, g *GameBoy) {
	t.Helper()
	for range 8*512/4 - 1 {
		step(g)
	}
	if g.Bus.ifl&IntSerial != 0 {
		t.Fatal("transfer completed early")
	}
	step(g)
	if g.Bus.ifl&IntSerial == 0 || g.Bus.read(0xFF02)&0x80 != 0 {
		t.Fatalf("transfer not completed: IF %02X SC %02X", g.Bus.ifl, g.Bus.read(0xFF02))
	}
}

func TestLinkMaster(t *testing.T) {
	g, p := newLinkedGB(t)
	seq := startMaster(t, g, p, 0x12)
	p.send(msgReply, seq, 0x42)
	finish(t, g)
	if sb := g.Bus.read(0xFF01); sb != 0x42 {
		t.Errorf("SB %02X, want 42 from the other side", sb)
	}
	if len(g.Serial.Output) != 0 {
		t.Errorf("output %q recorded with a cable plugged in", g.Serial.Output)
	}

	// The answer may come in late: the master waits for it.
	g.Bus.ifl = 0
	seq = startMaster(t, g, p, 0x13)
	go func() {
		time.Sleep(20 * time.Millisecond)
		p.send(msgReply, seq, 0x43)
	}()
	finish(t, g)
	if sb := g.Bus.read(0xFF01); sb != 0x43 {
		t.Errorf("late answer: SB %02X, want 43", sb)
	}
}

func TestLinkMasterTimeout(t *testing.T) {
	g, p := newLinkedGB(t)
	seq := startMaster(t, g, p, 0x12)
	begin := time.Now()
	finish(t, g)
	if waited := time.Since(begin); waited < linkTimeout {
		t.Errorf("waited %v, want %v", waited, linkTimeout)
	}
	if sb := g.Bus.read(0xFF01); sb != 0xFF {
		t.Errorf("SB %02X, want FF after a timeout", sb)
	}

	// The other side is stalled: no more waiting.
	g.Bus.ifl = 0
	startMaster(t, g, p, 0x13)
	begin = time.Now()
	finish(t, g)
	if sb := g.Bus.read(0xFF01); sb != 0xFF {
		t.Errorf("stalled: SB %02X, want FF", sb)
	}
	if waited := time.Since(begin); waited > linkTimeout/2 {
		t.Errorf("stalled: waited %v", waited)
	}

	// Until it sends something, here its late answer, which is not taken
	// for the answer to the next transfer.
	p.send(msgReply, seq, 0x42)
	g.Bus.ifl = 0
	seq = startMaster(t, g, p, 0x14)
	go func() {
		time.Sleep(20 * time.Millisecond)
		p.send(msgReply, seq, 0x44)
	}()
	finish(t, g)
	if sb := g.Bus.read(0xFF01); sb != 0x44 {
		t.Errorf("SB %02X, want 44: the late answer was to another transfer", sb)
	}
}

func TestLinkSlave(t *testing.T) {
	g, p := newLinkedGB(t)
	g.Bus.write(0xFF01, 0x34)
	g.Bus.write(0xFF02, 0x80)
	g.Serial.poll()
	// Answered at once, without the emulation running.
	p.send(msgTransfer, 7, 0x99)
	if seq := p.expect(msgReply, 0x34); seq != 7 {
		t.Errorf("answer to transfer %d, want 7", seq)
	}
	pollUntil(t, g)
	if sb, sc := g.Bus.read(0xFF01), g.Bus.read(0xFF02); sb != 0x99 || sc != 0x7E {
		t.Errorf("SB %02X SC %02X, want 99 7E", sb, sc)
	}

	// Disarmed now: the next one is held.
	g.Bus.ifl = 0
	p.send(msgTransfer, 8, 0x98)
	time.Sleep(10 * time.Millisecond)
	g.Serial.poll()
	p.expectNone()

	// Until the game arms the port.
	g.Bus.write(0xFF01, 0x35)
	g.Bus.write(0xFF02, 0x80)
	g.Serial.poll()
	if seq := p.expect(msgReply, 0x35); seq != 8 {
		t.Errorf("answer to transfer %d, want 8", seq)
	}
	if g.Bus.ifl&IntSerial == 0 || g.Bus.read(0xFF01) != 0x98 {
		t.Errorf("held transfer: IF %02X SB %02X, want the interrupt and 98", g.Bus.ifl, g.Bus.read(0xFF01))
	}
}

func TestLinkNotArmed(t *testing.T) {
	g, p := newLinkedGB(t)
	g.Bus.write(0xFF01, 0x34)
	p.send(msgTransfer, 1, 0x99)
	time.Sleep(10 * time.Millisecond)
	g.Serial.poll()
	g.Bus.cycles += linkWindow - 1
	g.Serial.poll()
	p.expectNone()
	g.Bus.cycles++
	g.Serial.poll()
	p.expect(msgReply, 0x00) // nothing shifted
	if g.Bus.ifl&IntSerial != 0 || g.Bus.read(0xFF01) != 0x34 {
		t.Errorf("IF %02X SB %02X, want no interrupt and 34", g.Bus.ifl, g.Bus.read(0xFF01))
	}
}

func TestLinkBothMasters(t *testing.T) {
	g, p := newLinkedGB(t)
	seq := startMaster(t, g, p, 0x12)
	// The other side started a transfer too: each shifts in 0xFF.
	p.send(msgTransfer, 3, 0x56)
	if s := p.expect(msgReply, 0xFF); s != 3 {
		t.Errorf("answer to transfer %d, want 3", s)
	}
	p.send(msgReply, seq, 0xFF)
	finish(t, g)
	if sb := g.Bus.read(0xFF01); sb != 0xFF {
		t.Errorf("SB %02X, want FF", sb)
	}
}

func TestLinkDisconnect(t *testing.T) {
	g, p := newLinkedGB(t)
	startMaster(t, g, p, 0x12)
	p.conn.Close()
	select {
	case <-g.Link().Done():
	case <-time.After(time.Second):
		t.Fatal("Done not closed")
	}
	begin := time.Now()
	finish(t, g)
	if sb := g.Bus.read(0xFF01); sb != 0xFF || time.Since(begin) > linkTimeout/2 {
		t.Errorf("SB %02X after %v, want FF at once", sb, time.Since(begin))
	}
}

func TestLinkReset(t *testing.T) {
	g, p := newLinkedGB(t)
	g.Reset()
	g.Bus.ifl = 0
	startMaster(t, g, p, 0x12)
	g.SetLink(nil)
	step(g)
	if sb := g.Bus.read(0xFF01); sb != 0xFF || g.Bus.ifl&IntSerial == 0 {
		t.Errorf("unplugged during the transfer: SB %02X IF %02X, want FF and the interrupt", sb, g.Bus.ifl)
	}
	g.Bus.write(0xFF01, 0x13)
	g.Bus.write(0xFF02, 0x81)
	if string(g.Serial.Output) != "\x13" {
		t.Errorf("output %q once unplugged, want 13", g.Serial.Output)
	}
}

// linkProgram exchanges 16 bytes, sending base+i and storing the bytes
// received at 0xC000. sc is 0x81 for the master, 0x80 for the slave.
func linkProgram(base, sc byte) []byte {
	return []byte{
		0x21, 0x00, 0xC0, // LD HL,0xC000
		0x06, 0x00, // LD B,0
		0x78,       // loop: LD A,B
		0xC6, base, // ADD A,base
		0xE0, 0x01, // LDH (SB),A
		0x3E, sc, // LD A,sc
		0xE0, 0x02, // LDH (SC),A
		0xF0, 0x02, // wait: LDH A,(SC)
		0xCB, 0x7F, // BIT 7,A
		0x20, 0xFA, // JR NZ,wait
		0xF0, 0x01, // LDH A,(SB)
		0x22,       // LD (HL+),A
		0x04,       // INC B
		0x78,       // LD A,B
		0xFE, 0x10, // CP 16
		0x20, 0xE8, // JR NZ,loop
		0x18, 0xFE, // JR $
	}
}

func TestLinkTwoGameBoys(t *testing.T) {
	master := newTestGB(t, linkProgram(0x10, 0x81)...)
	slave := newTestGB(t, linkProgram(0x80, 0x80)...)
	a, b := net.Pipe()
	master.SetLink(NewLink(a))
	slave.SetLink(NewLink(b))
	defer master.Link().Close()

	// Each runs at its own pace, the slave starting late.
	var wg sync.WaitGroup
	for i, g := range []*GameBoy{master, slave} {
		wg.Go(func() {
			if i == 1 {
				time.Sleep(30 * time.Millisecond)
			}
			for range 20 {
				g.RunLinkedFrame()
			}
		})
	}
	wg.Wait()
	for _, c := range []struct {
		name string
		g    *GameBoy
		base byte
	}{{"master", master, 0x80}, {"slave", slave, 0x10}} {
		for i := range 16 {
			if got, want := c.g.Bus.read(0xC000+uint16(i)), c.base+byte(i); got != want {
				t.Errorf("%s byte %d: %02X, want %02X", c.name, i, got, want)
			}
		}
	}
}

// TestLinkTetris starts a game of Tetris for two, each Game Boy running at
// its own pace: the first picks 2PLAYER on the title screen, then both go
// through the settings to the game.
func TestLinkTetris(t *testing.T) {
	var gs [2]*GameBoy
	for i := range gs {
		gs[i] = loadROMModel(t, filepath.Join(gameDir, "Tetris_World_Rev1.gb"), ModelDMG)
	}
	a, b := net.Pipe()
	gs[0].SetLink(NewLink(a))
	gs[1].SetLink(NewLink(b))
	defer gs[0].Link().Close()
	presses := [2]map[int]Button{
		{600: ButtonRight, 650: ButtonStart, 950: ButtonStart, 1100: ButtonStart},
		{1100: ButtonStart},
	}
	var wg sync.WaitGroup
	for i, g := range gs {
		wg.Go(func() {
			for f := range 1500 {
				var down [8]bool
				for at, b := range presses[i] {
					down[b] = down[b] || f >= at && f < at+5
				}
				for b, d := range down {
					g.SetButton(Button(b), d)
				}
				g.RunLinkedFrame()
			}
		})
	}
	wg.Wait()

	// The panel of the game for two ("HIGH 0", "LINES 30"), on both.
	panel := func(g *GameBoy) (rows [][]byte) {
		fb := g.Framebuffer()
		for y := 32; y < 96; y++ {
			rows = append(rows, fb[y*ScreenWidth+104:y*ScreenWidth+152])
		}
		return rows
	}
	p0, p1 := panel(gs[0]), panel(gs[1])
	shades := map[byte]bool{}
	for y := range p0 {
		if !bytes.Equal(p0[y], p1[y]) {
			t.Fatalf("row %d of the panel differs: the two are not in the same game", 32+y)
		}
		for _, s := range p0[y] {
			shades[s] = true
		}
	}
	if len(shades) < 2 {
		t.Fatal("blank panel")
	}
}

// TestLinkPeerNotReading checks that the emulation never waits for the
// network: the other side stopped reading (and answering) altogether.
func TestLinkPeerNotReading(t *testing.T) {
	g := newTestGB(t)
	a, b := net.Pipe()
	defer b.Close()
	g.SetLink(NewLink(a))
	defer g.Link().Close()
	begin := time.Now()
	for range 1000 {
		g.Bus.ifl = 0
		g.Bus.write(0xFF02, 0x81)
		g.Serial.poll()
		finish(t, g)
	}
	// One timeout, then the other side is known to be stalled.
	if d := time.Since(begin); d > linkTimeout+time.Second {
		t.Errorf("1000 transfers took %v", d)
	}
}
