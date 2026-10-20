package udp

import (
	"net"
	"net/netip"
	"time"
)

// Connection is the socket a Server reads its datagrams from: the subset of *net.UDPConn the server needs,
// so you can hand it a socket of your own with Server.WithConnection. The methods are named after the ones of
// *net.UDPConn, so a standard socket satisfies the interface as it is.
type Connection interface {
	// ReadFromUDPAddrPort reads one datagram into b, and returns its length and the address it was sent from.
	ReadFromUDPAddrPort(b []byte) (int, netip.AddrPort, error)

	// LocalAddr returns the local address the socket is bound to.
	LocalAddr() net.Addr

	// SetReadDeadline makes the pending and future reads fail with os.ErrDeadlineExceeded once t has passed.
	// The zero time removes the deadline.
	SetReadDeadline(t time.Time) error

	// Close closes the socket, which unblocks a pending read.
	Close() error
}

// *net.UDPConn implements the Connection interface.
var _ Connection = (*net.UDPConn)(nil)
