package fixture

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	httpserver "github.com/grandper/go-athanor/resource/server/http"
)

// HandlerPanic is what an OnHandlerPanicked hook received: the request whose handler panicked, and the value
// recovered from the panic.
type HandlerPanic struct {
	Request   *http.Request
	Recovered any
}

// HookRecorder records the lifecycle events an [httpserver.Server] reports to its hooks, in the order they
// happened. It is safe for concurrent use, since the server calls its hooks from the goroutines of net/http.
//
// Each event is recorded as a stable, human-readable text:
//
//   - "handler panicked: <method> <path>: <recovered>" when the handler of a request panics.
//   - "error logged: <first line of the error>" when net/http logs an error; the stack trace net/http logs
//     after a panic is left out of the event, and kept in ErrorsLogged.
//   - "connection status changed: <status>" when a client connection changes status; <status> is the text of the
//     http.ConnState, such as "new", "active", "idle", "hijacked" or "closed".
//   - "shutdown started" when the server starts shutting down.
//
// The events recorded through the hooks of HooksLabeled are prefixed with "<label>: ".
type HookRecorder struct {
	mu           sync.Mutex
	events       []string
	panics       []HandlerPanic
	errorsLogged []error
	connections  []net.Conn
}

// NewHookRecorder returns a HookRecorder that recorded nothing yet.
func NewHookRecorder() *HookRecorder {
	return &HookRecorder{}
}

// Hooks returns one recording hook per lifecycle event of the server.
func (r *HookRecorder) Hooks() []httpserver.Hook {
	return r.hooks("")
}

// HooksLabeled returns one recording hook per lifecycle event of the server, whose events are prefixed with
// "<label>: ". Give the same recorder several labeled sets of hooks to observe the order they run in.
func (r *HookRecorder) HooksLabeled(label string) []httpserver.Hook {
	return r.hooks(label + ": ")
}

// HandlerPanickedHook returns the recording OnHandlerPanicked hook alone.
func (r *HookRecorder) HandlerPanickedHook() httpserver.Hook {
	return r.handlerPanickedHook("")
}

// ErrorLoggedHook returns the recording OnErrorLogged hook alone.
func (r *HookRecorder) ErrorLoggedHook() httpserver.Hook {
	return r.errorLoggedHook("")
}

// ConnectionStatusChangedHook returns the recording OnConnectionStatusChanged hook alone.
func (r *HookRecorder) ConnectionStatusChangedHook() httpserver.Hook {
	return r.connectionStatusChangedHook("")
}

// ShutdownStartedHook returns the recording OnShutdownStarted hook alone.
func (r *HookRecorder) ShutdownStartedHook() httpserver.Hook {
	return r.shutdownStartedHook("")
}

// hooks returns the recording hooks, prefixing every event with prefix.
func (r *HookRecorder) hooks(prefix string) []httpserver.Hook {
	return []httpserver.Hook{
		r.handlerPanickedHook(prefix),
		r.errorLoggedHook(prefix),
		r.connectionStatusChangedHook(prefix),
		r.shutdownStartedHook(prefix),
	}
}

// connectionStatusChangedHook returns the recording OnConnectionStatusChanged hook, prefixing its event with
// prefix.
func (r *HookRecorder) connectionStatusChangedHook(prefix string) httpserver.Hook {
	return httpserver.OnConnectionStatusChanged(func(conn net.Conn, status http.ConnState) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.connections = append(r.connections, conn)
		r.events = append(r.events, prefix+"connection status changed: "+status.String())
	})
}

// shutdownStartedHook returns the recording OnShutdownStarted hook, prefixing its event with prefix.
func (r *HookRecorder) shutdownStartedHook(prefix string) httpserver.Hook {
	return httpserver.OnShutdownStarted(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.events = append(r.events, prefix+"shutdown started")
	})
}

// handlerPanickedHook returns the recording OnHandlerPanicked hook, prefixing its event with prefix.
func (r *HookRecorder) handlerPanickedHook(prefix string) httpserver.Hook {
	return httpserver.OnHandlerPanicked(func(request *http.Request, recovered any) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.panics = append(r.panics, HandlerPanic{Request: request, Recovered: recovered})
		r.events = append(r.events,
			fmt.Sprintf("%shandler panicked: %s %s: %v", prefix, request.Method, request.URL.Path, recovered))
	})
}

// errorLoggedHook returns the recording OnErrorLogged hook, prefixing its event with prefix.
func (r *HookRecorder) errorLoggedHook(prefix string) httpserver.Hook {
	return httpserver.OnErrorLogged(func(err error) {
		firstLine, _, _ := strings.Cut(err.Error(), "\n")

		r.mu.Lock()
		defer r.mu.Unlock()
		r.errorsLogged = append(r.errorsLogged, err)
		r.events = append(r.events, prefix+"error logged: "+firstLine)
	})
}

// Events returns a snapshot of the events recorded so far, in order.
func (r *HookRecorder) Events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

// EventsContaining returns a snapshot of the events recorded so far that contain text, in order. It isolates the
// events of one kind from those other goroutines of net/http report concurrently.
func (r *HookRecorder) EventsContaining(text string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var events []string
	for _, event := range r.events {
		if strings.Contains(event, text) {
			events = append(events, event)
		}
	}
	return events
}

// Connections returns a snapshot of the connections the OnConnectionStatusChanged hooks received so far, one per
// status change, in order.
func (r *HookRecorder) Connections() []net.Conn {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]net.Conn(nil), r.connections...)
}

// Panics returns a snapshot of what the OnHandlerPanicked hooks received so far, in order.
func (r *HookRecorder) Panics() []HandlerPanic {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]HandlerPanic(nil), r.panics...)
}

// ErrorsLogged returns a snapshot of the errors the OnErrorLogged hooks received so far, in order.
func (r *HookRecorder) ErrorsLogged() []error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]error(nil), r.errorsLogged...)
}

// WaitForEvents fails the test unless at least count events are recorded within respondsWithin,
// then returns a snapshot of the events recorded so far.
func (r *HookRecorder) WaitForEvents(t *testing.T, count int) []string {
	t.Helper()
	require.Eventually(t, func() bool { return len(r.Events()) >= count }, respondsWithin, respondsTick,
		"the hooks never recorded %d event(s)", count)
	return r.Events()
}
