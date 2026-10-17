package link

import (
	"bytes"
	"encoding/binary"
	"log"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A gbe waiting for the other one (Host) announces itself on the local
// network: every announceEvery, while not connected, it broadcasts a UDP
// datagram to DiscoveryPort. Browse listens for them. The datagram is
// hello, the TCP port (2 bytes, big-endian), then the name and the title
// of the game, each preceded by its length. The address to connect to is
// the one the datagram came from.

// DiscoveryPort is the UDP port of the announcements.
const DiscoveryPort = 5556

const (
	announceEvery = time.Second
	// forgetAfter is how long a host stays listed after its last
	// announcement.
	forgetAfter = 3 * announceEvery
)

// Found is a gbe found on the local network, waiting for another one.
type Found struct {
	Name, Title string
	Addr        string // host:port to connect to
}

// announce broadcasts the announcements of Host until Close.
func (c *Cable) announce() {
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		log.Printf("link: cannot announce on the local network: %v", err)
		return
	}
	defer conn.Close()
	tcp := c.ln.Addr().(*net.TCPAddr)
	dsts := announceAddrs(tcp.IP.IsLoopback())
	t := time.NewTicker(announceEvery)
	defer t.Stop()
	for {
		if !c.connected() {
			c.mu.Lock()
			msg := announcement(tcp.Port, c.name, c.title)
			c.mu.Unlock()
			for _, dst := range dsts {
				conn.WriteToUDP(msg, dst) // a network may refuse: the others are tried
			}
		}
		select {
		case <-t.C:
		case <-c.quit:
			return
		}
	}
}

func announcement(port int, name, title string) []byte {
	msg := binary.BigEndian.AppendUint16(slices.Clone(hello), uint16(port))
	msg = append(append(msg, byte(len(name))), name...)
	return append(append(msg, byte(len(title))), title...)
}

// parseAnnouncement reads an announcement, reporting whether it is one.
func parseAnnouncement(msg []byte) (port int, name, title string, ok bool) {
	rest, ok := bytes.CutPrefix(msg, hello)
	if !ok || len(rest) < 3 {
		return 0, "", "", false
	}
	port, rest = int(binary.BigEndian.Uint16(rest)), rest[2:]
	field := func() (string, bool) {
		if len(rest) == 0 || len(rest) < 1+int(rest[0]) {
			return "", false
		}
		s := string(rest[1 : 1+rest[0]])
		rest = rest[1+rest[0]:]
		return cleanName(s), true
	}
	if name, ok = field(); !ok {
		return 0, "", "", false
	}
	if title, ok = field(); !ok {
		return 0, "", "", false
	}
	return port, name, title, port != 0
}

// announceAddrs is where the announcements go: this machine, and unless
// the cable only listens there, the broadcast address of every network it
// is on (255.255.255.255 alone only reaches one of them).
func announceAddrs(loopbackOnly bool) []*net.UDPAddr {
	dsts := []*net.UDPAddr{{IP: net.IPv4(127, 0, 0, 1), Port: DiscoveryPort}}
	if loopbackOnly {
		return dsts
	}
	dsts = append(dsts, &net.UDPAddr{IP: net.IPv4bcast, Port: DiscoveryPort})
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil {
				continue
			}
			bcast := slices.Clone(n.IP.To4())
			for i := range bcast {
				bcast[i] |= ^n.Mask[len(n.Mask)-4+i]
			}
			dsts = append(dsts, &net.UDPAddr{IP: bcast, Port: DiscoveryPort})
		}
	}
	return dsts
}

// LocalAddrs are the IPv4 addresses of this machine on its networks, as
// the other gbe would type them.
func LocalAddrs() []string {
	var addrs []string
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		as, _ := ifc.Addrs()
		for _, a := range as {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLinkLocalUnicast() {
				addrs = append(addrs, n.IP.String())
			}
		}
	}
	return addrs
}

// Browser lists the gbe announcing themselves on the local network.
type Browser struct {
	conn  *net.UDPConn
	mu    sync.Mutex
	found map[string]seen // by name and port
	done  chan struct{}
	now   func() time.Time
}

type seen struct {
	Found
	at time.Time
}

// Browse listens for the announcements until Close. It fails when the
// port is taken, by another gbe looking for one.
func Browse() (*Browser, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: DiscoveryPort})
	if err != nil {
		return nil, err
	}
	b := &Browser{conn: conn, found: map[string]seen{}, done: make(chan struct{}), now: time.Now}
	go b.read()
	return b, nil
}

func (b *Browser) read() {
	defer close(b.done)
	buf := make([]byte, 512)
	for {
		n, from, err := b.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		b.add(buf[:n], from)
	}
}

// add records an announcement from the address from.
func (b *Browser) add(msg []byte, from *net.UDPAddr) {
	port, name, title, ok := parseAnnouncement(msg)
	if !ok {
		return
	}
	addr := net.JoinHostPort(from.IP.String(), strconv.Itoa(port))
	key := name + "|" + strconv.Itoa(port)
	b.mu.Lock()
	defer b.mu.Unlock()
	// The same gbe may be heard twice, through the network and through
	// this machine: its address on the network is kept, which works from
	// this machine too.
	if old, ok := b.found[key]; ok && from.IP.IsLoopback() && !strings.HasPrefix(old.Addr, "127.") &&
		b.now().Sub(old.at) < forgetAfter {
		old.Title, old.at = title, b.now()
		b.found[key] = old
		return
	}
	b.found[key] = seen{Found{Name: name, Title: title, Addr: addr}, b.now()}
}

// Games is the gbe heard lately, by name.
func (b *Browser) Games() []Found {
	b.mu.Lock()
	defer b.mu.Unlock()
	var list []Found
	for key, s := range b.found {
		if b.now().Sub(s.at) > forgetAfter {
			delete(b.found, key)
			continue
		}
		list = append(list, s.Found)
	}
	slices.SortFunc(list, func(a, b Found) int {
		return strings.Compare(a.Name+a.Addr, b.Name+b.Addr)
	})
	return list
}

// Close stops listening.
func (b *Browser) Close() {
	b.conn.Close()
	<-b.done
}
