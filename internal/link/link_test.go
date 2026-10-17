package link

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/ldechoux/gbe/internal/gb"
)

func next(t *testing.T, c *Cable) *gb.Link {
	t.Helper()
	select {
	case l := <-c.Links():
		return l
	case <-time.After(5 * time.Second):
		t.Fatal("no connection")
	}
	return nil
}

func TestCable(t *testing.T) {
	host, err := Host("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	guest := Connect(host.Addr().String())
	defer guest.Close()
	h, g := next(t, host), next(t, guest)

	// Unplugged on one side: done on both, then connected again.
	g.Close()
	for _, l := range []*gb.Link{h, g} {
		select {
		case <-l.Done():
		case <-time.After(time.Second):
			t.Fatal("link not done")
		}
	}
	next(t, host)
	next(t, guest)
}

func TestConnectWaitsForHost(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	guest := Connect(addr)
	defer guest.Close()
	time.Sleep(100 * time.Millisecond) // the first attempt fails
	host, err := Host(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	next(t, host)
	next(t, guest)
}

func TestHandshake(t *testing.T) {
	for _, c := range []struct{ other, err string }{
		{"GBE-LINK\x01", ""},
		{"GBE-LINK\x02", "version 2"},
		{"GET / HTTP", "not gbe"},
	} {
		a, b := net.Pipe()
		go func() {
			b.Write([]byte(c.other))
			b.Read(make([]byte, len(hello)))
		}()
		err := handshake(a)
		if c.err == "" && err != nil || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("other side %q: error %v, want %q", c.other, err, c.err)
		}
		a.Close()
		b.Close()
	}
}

func TestHostBusy(t *testing.T) {
	host, err := Host("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	if _, err := Host(host.Addr().String()); err == nil {
		t.Error("listening twice on the same port")
	}
}

func TestCloseWhileConnecting(t *testing.T) {
	c := Connect("192.0.2.1:5555") // TEST-NET-1: never answers
	time.Sleep(50 * time.Millisecond)
	begin := time.Now()
	c.Close()
	if d := time.Since(begin); d > time.Second {
		t.Errorf("Close took %v", d)
	}
}
