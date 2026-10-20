package fixture

import (
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/grandper/go-athanor/resource/server/udp"
)

// FailingConnection is a [udp.Connection] whose reads always fail with a configured error.
type FailingConnection struct {
	err error

	mu     sync.Mutex
	closed bool
}

// NewFailingConnection returns a FailingConnection failing every read with err.
func NewFailingConnection(err error) *FailingConnection {
	return &FailingConnection{err: err}
}

// ReadFromUDPAddrPort implements the udp.Connection interface. It always fails with the configured error.
func (c *FailingConnection) ReadFromUDPAddrPort([]byte) (int, netip.AddrPort, error) {
	return 0, netip.AddrPort{}, c.err
}

// LocalAddr implements the udp.Connection interface. It returns an unbound loopback UDP address.
func (c *FailingConnection) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP(loopbackAddress)}
}

// SetReadDeadline implements the udp.Connection interface. It accepts any deadline: the reads fail anyway.
func (c *FailingConnection) SetReadDeadline(time.Time) error {
	return nil
}

// Close implements the udp.Connection interface. It records that the connection was closed.
func (c *FailingConnection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

// WasClosed reports whether Close was called.
func (c *FailingConnection) WasClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// FailingConnection implements the udp.Connection interface.
var _ udp.Connection = (*FailingConnection)(nil)
