package fixture

import (
	"net"
	"sync"
)

// FailingListener is a [net.Listener] whose Accept always fails with a configured error.
type FailingListener struct {
	err error

	mu     sync.Mutex
	closed bool
}

// NewFailingListener returns a FailingListener failing every Accept with err.
func NewFailingListener(err error) *FailingListener {
	return &FailingListener{err: err}
}

// Accept implements the net.Listener interface. It always fails with the configured error.
func (l *FailingListener) Accept() (net.Conn, error) {
	return nil, l.err
}

// Close implements the net.Listener interface. It records that the listener was closed.
func (l *FailingListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	return nil
}

// Addr implements the net.Listener interface. It returns an unbound loopback TCP address.
func (l *FailingListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv6loopback}
}

// WasClosed reports whether Close was called.
func (l *FailingListener) WasClosed() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closed
}

// FailingListener implements the net.Listener interface.
var _ net.Listener = (*FailingListener)(nil)
