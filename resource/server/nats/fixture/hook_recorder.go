package fixture

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	natsserver "github.com/grandper/go-athanor/resource/server/nats"
)

// HandlerFailure is what an OnHandlerFailed hook received: the connection, the message the handler failed to
// process, and the error the handler returned.
type HandlerFailure struct {
	Conn *nats.Conn
	Msg  *nats.Msg
	Err  error
}

// HookRecorder records the lifecycle events a [natsserver.Server] reports to its hooks, in the order they
// happened. It is safe for concurrent use, since the server calls its hooks from the goroutines of the NATS
// client.
//
// Each event is recorded as a stable, human-readable text:
//
//   - "connection established" when the connection is reported established at start.
//   - "connection lost: <error>" when the connection is lost; <error> is "<nil>" when the client reports none.
//   - "connection restored" when the connection is restored.
//   - "connection closed" when the connection is closed for good.
//   - "subscription opened: <subject>" when the server opens its subscription to <subject>.
//   - "subscription status changed: <subject>: <status>" when that subscription changes status; <status> is the
//     text of the nats.SubStatus, such as "Closed" or "SlowConsumer".
//   - "subscription closed: <subject>" when that subscription closes.
//   - "handler failed: <subject>: <error>" when a handler fails to process a message published on <subject>.
//
// The events recorded through the hooks of HooksLabeled are prefixed with "<label>: ".
type HookRecorder struct {
	mu              sync.Mutex
	events          []string
	connections     []*nats.Conn
	subscriptions   []*nats.Subscription
	handlerFailures []HandlerFailure
}

// NewHookRecorder returns a HookRecorder that recorded nothing yet.
func NewHookRecorder() *HookRecorder {
	return &HookRecorder{}
}

// Hooks returns one recording hook per lifecycle event of the server.
func (r *HookRecorder) Hooks() []natsserver.Hook {
	return r.hooks("")
}

// HooksLabeled returns one recording hook per lifecycle event of the server, whose events are prefixed with
// "<label>: ". Give the same recorder several labeled sets of hooks to observe the order they run in.
func (r *HookRecorder) HooksLabeled(label string) []natsserver.Hook {
	return r.hooks(label + ": ")
}

// ConnectionHooks returns the recording hooks of the connection events alone.
func (r *HookRecorder) ConnectionHooks() []natsserver.Hook {
	return r.connectionHooks("")
}

// SubscriptionHooks returns the recording hooks of the subscription events alone.
func (r *HookRecorder) SubscriptionHooks() []natsserver.Hook {
	return r.subscriptionHooks("")
}

// ConnectionEstablishedHook returns the recording OnConnectionEstablished hook alone.
func (r *HookRecorder) ConnectionEstablishedHook() natsserver.Hook {
	return r.connectionEstablishedHook("")
}

// ConnectionLostHook returns the recording OnConnectionLost hook alone.
func (r *HookRecorder) ConnectionLostHook() natsserver.Hook {
	return r.connectionLostHook("")
}

// ConnectionRestoredHook returns the recording OnConnectionRestored hook alone.
func (r *HookRecorder) ConnectionRestoredHook() natsserver.Hook {
	return r.connectionRestoredHook("")
}

// ConnectionClosedHook returns the recording OnConnectionClosed hook alone.
func (r *HookRecorder) ConnectionClosedHook() natsserver.Hook {
	return r.connectionClosedHook("")
}

// SubscriptionOpenedHook returns the recording OnSubscriptionOpened hook alone.
func (r *HookRecorder) SubscriptionOpenedHook() natsserver.Hook {
	return r.subscriptionOpenedHook("")
}

// SubscriptionStatusChangedHook returns the recording OnSubscriptionStatusChanged hook alone.
func (r *HookRecorder) SubscriptionStatusChangedHook() natsserver.Hook {
	return r.subscriptionStatusChangedHook("")
}

// SubscriptionClosedHook returns the recording OnSubscriptionClosed hook alone.
func (r *HookRecorder) SubscriptionClosedHook() natsserver.Hook {
	return r.subscriptionClosedHook("")
}

// HandlerFailedHook returns the recording OnHandlerFailed hook alone.
func (r *HookRecorder) HandlerFailedHook() natsserver.Hook {
	return r.handlerFailedHook("")
}

// hooks returns the recording hooks, prefixing every event with prefix.
func (r *HookRecorder) hooks(prefix string) []natsserver.Hook {
	hooks := append(r.connectionHooks(prefix), r.subscriptionHooks(prefix)...)
	return append(hooks, r.handlerFailedHook(prefix))
}

// connectionHooks returns the recording hooks of the connection events, prefixing every event with prefix.
func (r *HookRecorder) connectionHooks(prefix string) []natsserver.Hook {
	return []natsserver.Hook{
		r.connectionEstablishedHook(prefix),
		r.connectionLostHook(prefix),
		r.connectionRestoredHook(prefix),
		r.connectionClosedHook(prefix),
	}
}

// subscriptionHooks returns the recording hooks of the subscription events, prefixing every event with prefix.
func (r *HookRecorder) subscriptionHooks(prefix string) []natsserver.Hook {
	return []natsserver.Hook{
		r.subscriptionOpenedHook(prefix),
		r.subscriptionStatusChangedHook(prefix),
		r.subscriptionClosedHook(prefix),
	}
}

// connectionEstablishedHook returns the recording OnConnectionEstablished hook, prefixing its event with prefix.
func (r *HookRecorder) connectionEstablishedHook(prefix string) natsserver.Hook {
	return natsserver.OnConnectionEstablished(func(nc *nats.Conn) {
		r.recordConnection(nc, prefix+"connection established")
	})
}

// connectionLostHook returns the recording OnConnectionLost hook, prefixing its event with prefix.
func (r *HookRecorder) connectionLostHook(prefix string) natsserver.Hook {
	return natsserver.OnConnectionLost(func(nc *nats.Conn, err error) {
		r.recordConnection(nc, fmt.Sprintf("%sconnection lost: %v", prefix, err))
	})
}

// connectionRestoredHook returns the recording OnConnectionRestored hook, prefixing its event with prefix.
func (r *HookRecorder) connectionRestoredHook(prefix string) natsserver.Hook {
	return natsserver.OnConnectionRestored(func(nc *nats.Conn) {
		r.recordConnection(nc, prefix+"connection restored")
	})
}

// connectionClosedHook returns the recording OnConnectionClosed hook, prefixing its event with prefix.
func (r *HookRecorder) connectionClosedHook(prefix string) natsserver.Hook {
	return natsserver.OnConnectionClosed(func(nc *nats.Conn) {
		r.recordConnection(nc, prefix+"connection closed")
	})
}

// subscriptionOpenedHook returns the recording OnSubscriptionOpened hook, prefixing its event with prefix.
func (r *HookRecorder) subscriptionOpenedHook(prefix string) natsserver.Hook {
	return natsserver.OnSubscriptionOpened(func(_ *nats.Conn, sub *nats.Subscription) {
		r.recordSubscription(sub, prefix+"subscription opened: "+sub.Subject)
	})
}

// subscriptionStatusChangedHook returns the recording OnSubscriptionStatusChanged hook, prefixing its event with
// prefix.
func (r *HookRecorder) subscriptionStatusChangedHook(prefix string) natsserver.Hook {
	return natsserver.OnSubscriptionStatusChanged(func(_ *nats.Conn, sub *nats.Subscription, status nats.SubStatus) {
		r.recordSubscription(sub, fmt.Sprintf("%ssubscription status changed: %s: %s", prefix, sub.Subject, status))
	})
}

// subscriptionClosedHook returns the recording OnSubscriptionClosed hook, prefixing its event with prefix.
func (r *HookRecorder) subscriptionClosedHook(prefix string) natsserver.Hook {
	return natsserver.OnSubscriptionClosed(func(_ *nats.Conn, sub *nats.Subscription) {
		r.recordSubscription(sub, prefix+"subscription closed: "+sub.Subject)
	})
}

// handlerFailedHook returns the recording OnHandlerFailed hook, prefixing its event with prefix.
func (r *HookRecorder) handlerFailedHook(prefix string) natsserver.Hook {
	return natsserver.OnHandlerFailed(func(nc *nats.Conn, msg *nats.Msg, err error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.handlerFailures = append(r.handlerFailures, HandlerFailure{Conn: nc, Msg: msg, Err: err})
		r.events = append(r.events, fmt.Sprintf("%shandler failed: %s: %v", prefix, msg.Subject, err))
	})
}

// recordConnection records event, reported by a connection hook that received nc.
func (r *HookRecorder) recordConnection(nc *nats.Conn, event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connections = append(r.connections, nc)
	r.events = append(r.events, event)
}

// recordSubscription records event, reported by a subscription hook that received sub.
func (r *HookRecorder) recordSubscription(sub *nats.Subscription, event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscriptions = append(r.subscriptions, sub)
	r.events = append(r.events, event)
}

// Events returns a snapshot of the events recorded so far, in order.
func (r *HookRecorder) Events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

// Connections returns a snapshot of the connections the connection hooks received so far, one per connection
// event, in order.
func (r *HookRecorder) Connections() []*nats.Conn {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*nats.Conn(nil), r.connections...)
}

// EventsContaining returns a snapshot of the events recorded so far that contain text, in order. It isolates the
// events reported on one goroutine, such as the closed subscriptions, from those reported concurrently on others.
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

// Subscriptions returns a snapshot of the subscriptions the subscription hooks received so far, one per
// subscription event, in order.
func (r *HookRecorder) Subscriptions() []*nats.Subscription {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*nats.Subscription(nil), r.subscriptions...)
}

// HandlerFailures returns a snapshot of what the OnHandlerFailed hooks received so far, in order.
func (r *HookRecorder) HandlerFailures() []HandlerFailure {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]HandlerFailure(nil), r.handlerFailures...)
}

// WaitForEvents fails the test unless at least count events are recorded within waitTimeout,
// then returns a snapshot of the events recorded so far.
func (r *HookRecorder) WaitForEvents(t *testing.T, count int) []string {
	t.Helper()
	require.Eventually(t, func() bool { return len(r.Events()) >= count }, waitTimeout, pollInterval,
		"the hooks never recorded %d event(s)", count)
	return r.Events()
}
