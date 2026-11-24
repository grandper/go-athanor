package fixture

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource/server/udp"
)

const pollInterval = 5 * time.Millisecond

// HandlerFailure is what an OnHandlerFailed hook received: the datagram the handler failed to process, and the
// error the handler returned. The payload is copied when the hook runs, so it shows any change the handler made.
type HandlerFailure struct {
	Datagram udp.Datagram
	Err      error
}

// ErrorRecorder records the datagrams a [udp.Server] reports to its OnHandlerFailed hooks, in the order they
// happened. It is safe for concurrent use, since the server calls its hooks from its read loop.
//
// Each failure is recorded as a stable, human-readable event:
//
//   - "handler failed: <payload>: <error>", the payload quoted as a Go string.
//
// The events recorded through the hooks of HooksLabeled are prefixed with "<label>: ".
type ErrorRecorder struct {
	mu       sync.Mutex
	events   []string
	failures []HandlerFailure
}

// NewErrorRecorder returns an ErrorRecorder that recorded nothing yet.
func NewErrorRecorder() *ErrorRecorder {
	return &ErrorRecorder{}
}

// Hooks returns the recording hooks of the server.
func (r *ErrorRecorder) Hooks() []udp.Hook {
	return r.hooks("")
}

// HooksLabeled returns the recording hooks of the server, whose events are prefixed with "<label>: ". Give the
// same recorder several labeled sets of hooks to observe the order they run in.
func (r *ErrorRecorder) HooksLabeled(label string) []udp.Hook {
	return r.hooks(label + ": ")
}

// hooks returns the recording hooks, prefixing every event with prefix.
func (r *ErrorRecorder) hooks(prefix string) []udp.Hook {
	return []udp.Hook{
		udp.OnHandlerFailed(func(datagram udp.Datagram, err error) {
			recorded := datagram
			recorded.Payload = append([]byte(nil), datagram.Payload...)

			r.mu.Lock()
			defer r.mu.Unlock()
			r.failures = append(r.failures, HandlerFailure{Datagram: recorded, Err: err})
			r.events = append(r.events, fmt.Sprintf("%shandler failed: %q: %v", prefix, recorded.Payload, err))
		}),
	}
}

// Events returns a snapshot of the events recorded so far, in order.
func (r *ErrorRecorder) Events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

// Failures returns a snapshot of what the OnHandlerFailed hooks received so far, in order. The payloads are
// copied too, so changing them leaves the record untouched.
func (r *ErrorRecorder) Failures() []HandlerFailure {
	r.mu.Lock()
	defer r.mu.Unlock()
	failures := make([]HandlerFailure, 0, len(r.failures))
	for _, failure := range r.failures {
		failure.Datagram.Payload = append([]byte(nil), failure.Datagram.Payload...)
		failures = append(failures, failure)
	}
	return failures
}

// WaitForEvents fails the test unless at least count events are recorded within timeout,
// then returns a snapshot of the events recorded so far.
func (r *ErrorRecorder) WaitForEvents(t *testing.T, count int, timeout time.Duration) []string {
	t.Helper()
	require.Eventually(t, func() bool { return len(r.Events()) >= count }, timeout, pollInterval,
		"the hooks never recorded %d event(s)", count)
	return r.Events()
}

// WaitForFailures fails the test unless at least count failures are recorded within timeout,
// then returns a snapshot of the failures recorded so far.
func (r *ErrorRecorder) WaitForFailures(t *testing.T, count int, timeout time.Duration) []HandlerFailure {
	t.Helper()
	require.Eventually(t, func() bool { return len(r.Failures()) >= count }, timeout, pollInterval,
		"the hooks never recorded %d failure(s)", count)
	return r.Failures()
}
