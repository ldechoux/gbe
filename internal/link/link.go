// Package link carries the link cable between two gbe over TCP: one waits
// for the other to connect (Host), the other connects to it (Connect). Both
// keep the cable up, connecting again after a disconnection.
package link

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/ldechoux/gbe/internal/gb"
)

// hello opens the connection both ways: the protocol and its version, which
// must match.
var hello = []byte("GBE-LINK\x01")

const (
	handshakeTimeout = 5 * time.Second
	retryDelay       = time.Second
)

// Cable is one end of the link cable. Each connection made is handed over
// by Links, as a gb.Link to plug into the Game Boy.
type Cable struct {
	links chan *gb.Link
	quit  chan struct{}
	once  sync.Once
	ln    net.Listener // Host only
	wg    sync.WaitGroup
}

// Host waits for the other gbe to connect on addr (host:port, or :port for
// every interface). It fails at once if it cannot listen there.
func Host(addr string) (*Cable, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	c := newCable()
	c.ln = ln
	c.run(func() (net.Conn, error) { return ln.Accept() })
	return c, nil
}

// Connect connects to the gbe waiting on addr (host:port), trying again
// until it answers.
func Connect(addr string) *Cable {
	c := newCable()
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

func newCable() *Cable {
	return &Cable{links: make(chan *gb.Link), quit: make(chan struct{})}
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
			if err == nil {
				if err = handshake(conn); err != nil {
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
			select {
			case c.links <- l:
			case <-c.quit:
				l.Close()
				return
			}
			select {
			case <-l.Done():
			case <-c.quit:
				l.Close()
				return
			}
		}
	})
}

// handshake checks that the other side is a gbe speaking the same protocol.
func handshake(conn net.Conn) error {
	if tcp, ok := conn.(*net.TCPConn); ok {
		tcp.SetNoDelay(true) // a transfer is a few bytes each way
	}
	conn.SetDeadline(time.Now().Add(handshakeTimeout))
	defer conn.SetDeadline(time.Time{})
	errc := make(chan error, 1)
	go func() {
		_, err := conn.Write(hello)
		errc <- err
	}()
	got := make([]byte, len(hello))
	if _, err := io.ReadFull(conn, got); err != nil {
		return fmt.Errorf("link: %v", err)
	}
	if !bytes.Equal(got, hello) { // the caller closes the connection
		if bytes.Equal(got[:len(hello)-1], hello[:len(hello)-1]) {
			return fmt.Errorf("link: the other gbe speaks version %d of the protocol, not %d", got[len(hello)-1], hello[len(hello)-1])
		}
		return fmt.Errorf("link: %v is not gbe", conn.RemoteAddr())
	}
	if err := <-errc; err != nil {
		return fmt.Errorf("link: %v", err)
	}
	return nil
}
