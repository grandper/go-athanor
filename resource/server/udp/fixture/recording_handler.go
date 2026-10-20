// Package fixture provides reusable testing objects for the UDP server package.
package fixture

import (
	"context"
	"sync"
	"time"

	athanorfixture "github.com/grandper/go-athanor/fixture"
	"github.com/grandper/go-athanor/resource/server/udp"
)

// recordedDatagrams is the number of datagrams WaitForDatagram can lag behind the handler before it drops them.
const recordedDatagrams = 1024

// RecordingHandler is a configurable [udp.Handler] for tests that records the datagrams it handles.
type RecordingHandler struct {
	mu         sync.Mutex
	datagrams  []udp.Datagram
	handleFunc func(ctx context.Context, datagram udp.Datagram) error
	handleErr  error
	received   chan udp.Datagram
}

// NewRecordingHandler returns a RecordingHandler accepting every datagram.
func NewRecordingHandler() *RecordingHandler {
	return &RecordingHandler{received: make(chan udp.Datagram, recordedDatagrams)}
}

// WithHandleFunc makes Handle delegate to fn once the datagram is recorded.
func (h *RecordingHandler) WithHandleFunc(
	fn func(ctx context.Context, datagram udp.Datagram) error,
) *RecordingHandler {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handleFunc = fn
	return h
}

// FailWith makes Handle fail with err once the datagram is recorded.
func (h *RecordingHandler) FailWith(err error) *RecordingHandler {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handleErr = err
	return h
}

// Handle implements the udp.Handler interface. It records a copy of the datagram, then fails with the
// configured error or delegates to the configured function.
func (h *RecordingHandler) Handle(ctx context.Context, datagram udp.Datagram) error {
	recorded := datagram
	recorded.Payload = append([]byte(nil), datagram.Payload...)

	h.mu.Lock()
	h.datagrams = append(h.datagrams, recorded)
	handleFunc, handleErr := h.handleFunc, h.handleErr
	h.mu.Unlock()

	select {
	case h.received <- recorded:
	default:
		// Nobody waits for that many datagrams: the record keeps them all anyway.
	}

	if handleErr != nil {
		return handleErr
	}
	if handleFunc != nil {
		return handleFunc(ctx, datagram)
	}
	return nil
}

// Datagrams returns a copy of the datagrams handled so far.
func (h *RecordingHandler) Datagrams() []udp.Datagram {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]udp.Datagram(nil), h.datagrams...)
}

// Calls returns the number of datagrams handled so far.
func (h *RecordingHandler) Calls() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.datagrams)
}

// WaitForDatagram returns the next datagram handled, or fails the test when none arrives within timeout.
func (h *RecordingHandler) WaitForDatagram(t athanorfixture.TestingT, timeout time.Duration) udp.Datagram {
	t.Helper()
	return athanorfixture.Receive(t, h.received, timeout, "the handler received no datagram")
}

// RecordingHandler implements the udp.Handler interface.
var _ udp.Handler = (*RecordingHandler)(nil)
