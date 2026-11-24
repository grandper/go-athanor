package fixture

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"

	grpcserver "github.com/grandper/go-athanor/resource/server/grpc"
)

// HandlerFailure is what an OnHandlerFailed hook received: the context of the request, the full method name, and
// the error the handler returned.
type HandlerFailure struct {
	Ctx    context.Context //nolint:containedctx // The recorder keeps what the hook received, for later checks.
	Method string
	Err    error
}

// HandlerPanic is what an OnHandlerPanicked hook received: the context of the request, the full method name, and
// the value recovered from the panic.
type HandlerPanic struct {
	Ctx       context.Context //nolint:containedctx // The recorder keeps what the hook received, for later checks.
	Method    string
	Recovered any
}

// HookRecorder records the lifecycle events a [grpcserver.Server] reports to its hooks, in the order they
// happened. It is safe for concurrent use, since the server calls its hooks from the goroutines of gRPC.
//
// Each event is recorded as a stable, human-readable text:
//
//   - "handler failed: <method>: <code>" when the handler of a request returns an error; <code> is the text of
//     its gRPC code, such as "NotFound" or "Internal".
//   - "handler panicked: <method>: <recovered>" when the handler of a request panics.
//   - "connection established" and "connection closed" when a client connection is established or closes; the
//     remote address varies from run to run, so it is left out of the event and kept in ConnectionInfos.
//   - "request stats reported: <type>" for each statistics event of a request; <type> is the Go type of the
//     event, such as "*stats.Begin" or "*stats.End".
//
// The events recorded through the hooks of HooksLabeled are prefixed with "<label>: ".
type HookRecorder struct {
	mu       sync.Mutex
	events   []string
	failures []HandlerFailure
	panics   []HandlerPanic
	conns    []*stats.ConnTagInfo
	rpcStats []stats.RPCStats
}

// NewHookRecorder returns a HookRecorder that recorded nothing yet.
func NewHookRecorder() *HookRecorder {
	return &HookRecorder{}
}

// Hooks returns one recording hook per lifecycle event of the server.
func (r *HookRecorder) Hooks() []grpcserver.Hook {
	return r.hooks("")
}

// HooksLabeled returns one recording hook per lifecycle event of the server, whose events are prefixed with
// "<label>: ". Give the same recorder several labeled sets of hooks to observe the order they run in.
func (r *HookRecorder) HooksLabeled(label string) []grpcserver.Hook {
	return r.hooks(label + ": ")
}

// HandlerFailedHook returns the recording OnHandlerFailed hook alone.
func (r *HookRecorder) HandlerFailedHook() grpcserver.Hook {
	return r.handlerFailedHook("")
}

// HandlerPanickedHook returns the recording OnHandlerPanicked hook alone.
func (r *HookRecorder) HandlerPanickedHook() grpcserver.Hook {
	return r.handlerPanickedHook("")
}

// ConnectionEstablishedHook returns the recording OnConnectionEstablished hook alone.
func (r *HookRecorder) ConnectionEstablishedHook() grpcserver.Hook {
	return r.connectionEstablishedHook("")
}

// ConnectionClosedHook returns the recording OnConnectionClosed hook alone.
func (r *HookRecorder) ConnectionClosedHook() grpcserver.Hook {
	return r.connectionClosedHook("")
}

// RequestStatsReportedHook returns the recording OnRequestStatsReported hook alone.
func (r *HookRecorder) RequestStatsReportedHook() grpcserver.Hook {
	return r.requestStatsReportedHook("")
}

// hooks returns the recording hooks, prefixing every event with prefix.
func (r *HookRecorder) hooks(prefix string) []grpcserver.Hook {
	return []grpcserver.Hook{
		r.handlerFailedHook(prefix),
		r.handlerPanickedHook(prefix),
		r.connectionEstablishedHook(prefix),
		r.connectionClosedHook(prefix),
		r.requestStatsReportedHook(prefix),
	}
}

// connectionEstablishedHook returns the recording OnConnectionEstablished hook, prefixing its event with prefix.
func (r *HookRecorder) connectionEstablishedHook(prefix string) grpcserver.Hook {
	return grpcserver.OnConnectionEstablished(func(_ context.Context, info *stats.ConnTagInfo) {
		r.recordConnection(info, prefix+"connection established")
	})
}

// connectionClosedHook returns the recording OnConnectionClosed hook, prefixing its event with prefix.
func (r *HookRecorder) connectionClosedHook(prefix string) grpcserver.Hook {
	return grpcserver.OnConnectionClosed(func(_ context.Context, info *stats.ConnTagInfo) {
		r.recordConnection(info, prefix+"connection closed")
	})
}

// requestStatsReportedHook returns the recording OnRequestStatsReported hook, prefixing its event with prefix.
func (r *HookRecorder) requestStatsReportedHook(prefix string) grpcserver.Hook {
	return grpcserver.OnRequestStatsReported(func(_ context.Context, requestStats stats.RPCStats) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.rpcStats = append(r.rpcStats, requestStats)
		r.events = append(r.events, fmt.Sprintf("%srequest stats reported: %T", prefix, requestStats))
	})
}

// recordConnection records event, reported by a connection hook that received info.
func (r *HookRecorder) recordConnection(info *stats.ConnTagInfo, event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns = append(r.conns, info)
	r.events = append(r.events, event)
}

// handlerFailedHook returns the recording OnHandlerFailed hook, prefixing its event with prefix.
func (r *HookRecorder) handlerFailedHook(prefix string) grpcserver.Hook {
	return grpcserver.OnHandlerFailed(func(ctx context.Context, method string, err error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.failures = append(r.failures, HandlerFailure{Ctx: ctx, Method: method, Err: err})
		r.events = append(r.events, fmt.Sprintf("%shandler failed: %s: %s", prefix, method, status.Code(err)))
	})
}

// handlerPanickedHook returns the recording OnHandlerPanicked hook, prefixing its event with prefix.
func (r *HookRecorder) handlerPanickedHook(prefix string) grpcserver.Hook {
	return grpcserver.OnHandlerPanicked(func(ctx context.Context, method string, recovered any) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.panics = append(r.panics, HandlerPanic{Ctx: ctx, Method: method, Recovered: recovered})
		r.events = append(r.events, fmt.Sprintf("%shandler panicked: %s: %v", prefix, method, recovered))
	})
}

// Events returns a snapshot of the events recorded so far, in order.
func (r *HookRecorder) Events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

// EventsContaining returns a snapshot of the events recorded so far that contain text, in order. It isolates the
// events of one kind from those other goroutines of gRPC report concurrently.
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

// ConnectionInfos returns a snapshot of what the connection hooks received so far, one per connection event, in
// order.
func (r *HookRecorder) ConnectionInfos() []*stats.ConnTagInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*stats.ConnTagInfo(nil), r.conns...)
}

// RequestStats returns a snapshot of the statistics events the OnRequestStatsReported hooks received so far, in
// order.
func (r *HookRecorder) RequestStats() []stats.RPCStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]stats.RPCStats(nil), r.rpcStats...)
}

// Failures returns a snapshot of what the OnHandlerFailed hooks received so far, in order.
func (r *HookRecorder) Failures() []HandlerFailure {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]HandlerFailure(nil), r.failures...)
}

// Panics returns a snapshot of what the OnHandlerPanicked hooks received so far, in order.
func (r *HookRecorder) Panics() []HandlerPanic {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]HandlerPanic(nil), r.panics...)
}

// WaitForEvents fails the test unless at least count events are recorded within servingWithin,
// then returns a snapshot of the events recorded so far.
func (r *HookRecorder) WaitForEvents(t *testing.T, count int) []string {
	t.Helper()
	require.Eventually(t, func() bool { return len(r.Events()) >= count }, servingWithin, servingTick,
		"the hooks never recorded %d event(s)", count)
	return r.Events()
}

// WaitForEventsContaining fails the test unless at least count events containing text are recorded within
// servingWithin, then returns a snapshot of the events containing text recorded so far.
func (r *HookRecorder) WaitForEventsContaining(t *testing.T, text string, count int) []string {
	t.Helper()
	require.Eventually(t, func() bool { return len(r.EventsContaining(text)) >= count }, servingWithin, servingTick,
		"the hooks never recorded %d event(s) containing %q", count, text)
	return r.EventsContaining(text)
}
