package link

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestAnnouncement(t *testing.T) {
	msg := announcement(5555, "mac", "TETRIS")
	if port, name, title, ok := parseAnnouncement(msg); !ok || port != 5555 || name != "mac" || title != "TETRIS" {
		t.Errorf("parsed %d %q %q %v", port, name, title, ok)
	}
	for _, bad := range [][]byte{nil, []byte("GBE-LINK\x02"), msg[:len(msg)-1], announcement(0, "mac", ""), []byte("hello")} {
		if _, _, _, ok := parseAnnouncement(bad); ok {
			t.Errorf("%q taken for an announcement", bad)
		}
	}
}

// A host is found by Browse, with its name and game, at an address that
// connects to it; it is no longer announced once connected.
func TestDiscovery(t *testing.T) {
	b, err := Browse()
	if err != nil {
		t.Skipf("discovery port taken: %v", err)
	}
	defer b.Close()
	host, err := Host("127.0.0.1:0", "mac")
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	host.SetTitle("TETRIS")

	var found Found
	eventually3(t, "the host is found", func() bool {
		for _, f := range b.Games() {
			if f.Name == "mac" && f.Title == "TETRIS" {
				found = f
				return true
			}
		}
		return false
	})
	guest := Connect(found.Addr, "guest")
	defer guest.Close()
	next(t, host)
	next(t, guest)
}

// The browser forgets a host that is no longer announced, and keeps the
// address on the network of one also heard through this machine.
func TestBrowserList(t *testing.T) {
	now := time.Unix(0, 0)
	b := &Browser{found: map[string]seen{}, now: func() time.Time { return now }}
	lan := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 20), Port: 50000}
	loop := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 50000}
	b.add(announcement(5555, "mac", "TETRIS"), lan)
	b.add(announcement(5555, "mac", "DR.MARIO"), loop)
	b.add(announcement(5555, "pc", ""), loop)
	b.add([]byte("noise"), lan)
	got := b.Games()
	want := []Found{{"mac", "DR.MARIO", "192.168.1.20:5555"}, {"pc", "", "127.0.0.1:5555"}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("games %+v, want %+v", got, want)
	}
	now = now.Add(forgetAfter + time.Millisecond)
	if got := b.Games(); len(got) != 0 {
		t.Errorf("games %+v, want none after %v", got, forgetAfter)
	}
}

// eventually3 is eventually with time for a few announcements.
func eventually3(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal(what + ": not after 3 s")
		}
	}
}

// The announcements of a host listening on every interface go to this
// machine and to the broadcast address of each network; one listening on
// this machine only, to this machine only.
func TestAnnounceAddrs(t *testing.T) {
	if got := announceAddrs(true); len(got) != 1 || !got[0].IP.IsLoopback() || got[0].Port != DiscoveryPort {
		t.Errorf("loopback only: %v", got)
	}
	all := announceAddrs(false)
	if len(all) < 2 || !all[0].IP.IsLoopback() || !all[1].IP.Equal(net.IPv4bcast) {
		t.Fatalf("every interface: %v", all)
	}
	for _, a := range LocalAddrs() {
		ip := net.ParseIP(a)
		if ip == nil || ip.To4() == nil || ip.IsLoopback() {
			t.Errorf("local address %q", a)
		}
	}
}

func TestNames(t *testing.T) {
	if n := LocalName(); n == "" || strings.Contains(n, ".") || len(n) > maxName {
		t.Errorf("local name %q", n)
	}
	for in, want := range map[string]string{
		"  Laurent's Mac ":      "Laurent's Mac",
		"Café\x00":              "Caf",
		strings.Repeat("a", 40): strings.Repeat("a", maxName),
	} {
		if got := cleanName(in); got != want {
			t.Errorf("cleanName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The two ends tell what they are: one hosts, on a port; the other
// connects, to an address.
func TestCableEnds(t *testing.T) {
	host, err := Host("127.0.0.1:0", "mac")
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	guest := Connect(host.Addr().String(), "pc")
	defer guest.Close()
	if !host.Hosting() || host.Port() == 0 || host.Target() != "" {
		t.Errorf("host: hosting %v, port %d, target %q", host.Hosting(), host.Port(), host.Target())
	}
	if guest.Hosting() || guest.Port() != 0 || guest.Addr() != nil || guest.Target() != host.Addr().String() {
		t.Errorf("guest: hosting %v, port %d, target %q", guest.Hosting(), guest.Port(), guest.Target())
	}
}
