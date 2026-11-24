package fixture

import (
	"fmt"
	"sync"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// ConnectCallbackRecorder records the connection callbacks given to nats.Connect, in the order they ran.
// It is safe for concurrent use, since the NATS client calls them on its own callback goroutine.
//
// Each callback is recorded as a stable, human-readable text:
//
//   - "disconnected: <error>" for the nats.DisconnectErrHandler; <error> is "<nil>" when the client reports none.
//   - "reconnected" for the nats.ReconnectHandler.
//   - "closed" for the nats.ClosedHandler.
type ConnectCallbackRecorder struct {
	mu     sync.Mutex
	events []string
}

// NewConnectCallbackRecorder returns a ConnectCallbackRecorder that recorded nothing yet.
func NewConnectCallbackRecorder() *ConnectCallbackRecorder {
	return &ConnectCallbackRecorder{}
}

// Options returns the nats.Connect options installing the recording disconnect, reconnect and closed callbacks.
func (r *ConnectCallbackRecorder) Options() []nats.Option {
	return []nats.Option{
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) { r.record(fmt.Sprintf("disconnected: %v", err)) }),
		nats.ReconnectHandler(func(*nats.Conn) { r.record("reconnected") }),
		nats.ClosedHandler(func(*nats.Conn) { r.record("closed") }),
	}
}

// record appends event to the recorded events.
func (r *ConnectCallbackRecorder) record(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

// Events returns a snapshot of the callbacks recorded so far, in order.
func (r *ConnectCallbackRecorder) Events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

// WaitForEvents fails the test unless at least count callbacks are recorded within waitTimeout,
// then returns a snapshot of the callbacks recorded so far.
func (r *ConnectCallbackRecorder) WaitForEvents(t *testing.T, count int) []string {
	t.Helper()
	require.Eventually(t, func() bool { return len(r.Events()) >= count }, waitTimeout, pollInterval,
		"the nats.Connect callbacks never recorded %d event(s)", count)
	return r.Events()
}
