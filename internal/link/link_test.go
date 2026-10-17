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
	host, err := Host("127.0.0.1:0", "host")
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	guest := Connect(host.Addr().String(), "guest")
	defer guest.Close()
	h, g := next(t, host), next(t, guest)
	if host.Peer() != "guest" || guest.Peer() != "host" {
		t.Errorf("peers %q and %q", host.Peer(), guest.Peer())
	}

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

	// The guest gone for good: the host forgets it.
	guest.Close()
	eventually(t, "the host forgets its peer", func() bool { return host.Peer() == "" })
}

func TestConnectWaitsForHost(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	guest := Connect(addr, "guest")
	defer guest.Close()
	time.Sleep(100 * time.Millisecond) // the first attempt fails
	host, err := Host(addr, "host")
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	next(t, host)
	next(t, guest)
}

func TestHandshake(t *testing.T) {
	for _, c := range []struct{ other, peer, err string }{
		{"GBE-LINK\x02\x03bob", "bob", ""},
		{"GBE-LINK\x02\x05b\xc3\xa9b\x01", "bb", ""}, // ASCII only
		{"GBE-LINK\x01\x00", "", "version 1"},
		{"GET / HTTP/1.1", "", "not gbe"},
	} {
		a, b := net.Pipe()
		go func() {
			b.Write([]byte(c.other))
			b.Read(make([]byte, 64))
		}()
		peer, err := handshake(a, "me")
		if c.err == "" && (err != nil || peer != c.peer) || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("other side %q: peer %q, error %v; want %q, %q", c.other, peer, err, c.peer, c.err)
		}
		a.Close()
		b.Close()
	}
}

func TestHostBusy(t *testing.T) {
	host, err := Host("127.0.0.1:0", "host")
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	if _, err := Host(host.Addr().String(), "again"); err == nil {
		t.Error("listening twice on the same port")
	}
}

func TestCloseWhileConnecting(t *testing.T) {
	c := Connect("192.0.2.1:5555", "guest") // TEST-NET-1: never answers
	time.Sleep(50 * time.Millisecond)
	begin := time.Now()
	c.Close()
	if d := time.Since(begin); d > time.Second {
		t.Errorf("Close took %v", d)
	}
}

// eventually waits for cond, failing after a second.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(time.Second); !cond(); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal(what + ": not after a second")
		}
	}
}
