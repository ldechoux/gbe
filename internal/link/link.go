// Package link carries the link cable between two gbe over TCP: one waits
// for the other to connect (Host), the other connects to it (Connect). Both
// keep the cable up, connecting again after a disconnection. The one that
// waits makes itself known on the local network, where Browse finds it
// (see discovery.go).
package link

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ldechoux/gbe/internal/gb"
)

// hello opens the connection both ways: the protocol and its version, which
// must match. Then each side sends its name: its length, then the name.
var hello = []byte("GBE-LINK\x02")

// DefaultPort is the TCP port the cable uses unless told otherwise.
const DefaultPort = 5555

const (
	handshakeTimeout = 5 * time.Second
	retryDelay       = time.Second
	maxName          = 32 // characters of a name, sent or shown
)

// Cable is one end of the link cable. Each connection made is handed over
// by Links, as a gb.Link to plug into the Game Boy.
type Cable struct {
	links  chan *gb.Link
	quit   chan struct{}
	once   sync.Once
	ln     net.Listener // Host only
	wg     sync.WaitGroup
	name   string // of this gbe, told to the other one
	target string // the address Connect connects to

	mu    sync.Mutex
	peer  string // the name of the other gbe, while connected
	title string // of the game being played, which Host announces
	up    bool   // connected
}

// Host waits for the other gbe to connect on addr (host:port, or :port for
// every interface), and announces itself on the local network meanwhile,
// as name. It fails at once if it cannot listen there.
func Host(addr, name string) (*Cable, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	c := newCable(name)
	c.ln = ln
	c.run(func() (net.Conn, error) { return ln.Accept() })
	c.wg.Go(c.announce)
	return c, nil
}

// Connect connects to the gbe waiting on addr (host:port), as name, trying
// again until it answers.
func Connect(addr, name string) *Cable {
	c := newCable(name)
	c.target = addr
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-c.quit
		cancel()
	}()
	d := net.Dialer{Timeout: handshakeTimeout}
	c.run(func() (net.Conn, error) { return d.DialContext(ctx, "tcp", addr) })
	return c
}

// Addr is the address Host listens on, nil for Connect.
func (c *Cable) Addr() net.Addr {
	if c.ln == nil {
		return nil
	}
	return c.ln.Addr()
}

// Hosting reports whether the cable waits for the other gbe (Host) rather
// than connecting to it.
func (c *Cable) Hosting() bool { return c.ln != nil }

// Port is the TCP port Host listens on, 0 for Connect.
func (c *Cable) Port() int {
	if c.ln == nil {
		return 0
	}
	return c.ln.Addr().(*net.TCPAddr).Port
}

// Target is the address Connect connects to, "" for Host.
func (c *Cable) Target() string { return c.target }

func newCable(name string) *Cable {
	return &Cable{links: make(chan *gb.Link), quit: make(chan struct{}), name: cleanName(name)}
}

// Peer is the name of the gbe at the other end, "" while not connected.
func (c *Cable) Peer() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.peer
}

// SetTitle sets the title of the game being played, which Host announces.
func (c *Cable) SetTitle(title string) {
	c.mu.Lock()
	c.title = cleanName(title)
	c.mu.Unlock()
}

// LocalName is the name of this machine, as the other gbe shows it.
func LocalName() string {
	name, err := os.Hostname()
	if err != nil {
		return "gbe"
	}
	name, _, _ = strings.Cut(name, ".") // without .local or the domain
	if name = cleanName(name); name == "" {
		return "gbe"
	}
	return name
}

// cleanName keeps the printable ASCII of s (the menu font has no other
// characters), at most maxName of them.
func cleanName(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if b.Len() == maxName {
			break
		}
		if r < unicode.MaxASCII && unicode.IsPrint(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Links hands over each connection made, one at a time: the next one is
// only made once the previous gb.Link is done.
func (c *Cable) Links() <-chan *gb.Link { return c.links }

// Close stops connecting, and ends the current connection.
func (c *Cable) Close() {
	c.once.Do(func() {
		close(c.quit)
		if c.ln != nil {
			c.ln.Close()
		}
	})
	c.wg.Wait()
}

func (c *Cable) run(connect func() (net.Conn, error)) {
	c.wg.Go(func() {
		for {
			conn, err := connect()
			var peer string
			if err == nil {
				if peer, err = handshake(conn, c.name); err != nil {
					log.Print(err)
				}
			}
			if err != nil {
				if conn != nil {
					conn.Close()
				}
				select { // the other side may not be there yet
				case <-time.After(retryDelay):
					continue
				case <-c.quit:
					return
				}
			}
			l := gb.NewLink(conn)
			c.setPeer(peer, true)
			select {
			case c.links <- l:
			case <-c.quit:
				l.Close()
				return
			}
			select {
			case <-l.Done():
				c.setPeer("", false)
			case <-c.quit:
				l.Close()
				return
			}
		}
	})
}

func (c *Cable) setPeer(name string, up bool) {
	c.mu.Lock()
	c.peer, c.up = name, up
	c.mu.Unlock()
}

// connected reports whether a connection is up.
func (c *Cable) connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.up
}

// handshake checks that the other side is a gbe speaking the same protocol,
// telling it our name and returning its own.
func handshake(conn net.Conn, name string) (string, error) {
	if tcp, ok := conn.(*net.TCPConn); ok {
		tcp.SetNoDelay(true) // a transfer is a few bytes each way
	}
	conn.SetDeadline(time.Now().Add(handshakeTimeout))
	defer conn.SetDeadline(time.Time{})
	errc := make(chan error, 1)
	go func() {
		msg := append(append(slices.Clone(hello), byte(len(name))), name...)
		_, err := conn.Write(msg)
		errc <- err
	}()
	got := make([]byte, len(hello)+1)
	if _, err := io.ReadFull(conn, got); err != nil {
		return "", fmt.Errorf("link: %v", err)
	}
	if !bytes.Equal(got[:len(hello)], hello) { // the caller closes the connection
		if bytes.Equal(got[:len(hello)-1], hello[:len(hello)-1]) {
			return "", fmt.Errorf("link: the other gbe speaks version %d of the protocol, not %d", got[len(hello)-1], hello[len(hello)-1])
		}
		return "", fmt.Errorf("link: %v is not gbe", conn.RemoteAddr())
	}
	peer := make([]byte, got[len(hello)])
	if _, err := io.ReadFull(conn, peer); err != nil {
		return "", fmt.Errorf("link: %v", err)
	}
	if err := <-errc; err != nil {
		return "", fmt.Errorf("link: %v", err)
	}
	return cleanName(string(peer)), nil
}
