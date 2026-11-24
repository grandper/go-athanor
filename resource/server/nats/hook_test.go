package nats_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource"
	natsserver "github.com/grandper/go-athanor/resource/server/nats"
	"github.com/grandper/go-athanor/resource/server/nats/fixture"
)

// quietPeriod is how long a test waits to be confident that a hook does not fire.
const quietPeriod = 100 * time.Millisecond

func TestOnHandlerFailed(t *testing.T) {
	ctx := context.Background()

	t.Run("should receive the connection, the failing message, and the error", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		errHandle := errors.New("the order has no line")
		handler := fixture.NewFakeHandler().
			WithHandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().FailWith(errHandle).Handle)
		server := natsserver.NewServer(nc, handler).WithHooks(recorder.HandlerFailedHook())
		require.NoError(t, server.Init(ctx))

		require.NoError(t, nc.Publish("orders.created", []byte("order-1")))

		assert.Equal(t, []string{"handler failed: orders.created: the order has no line"},
			recorder.WaitForEvents(t, 1))
		failure := recorder.HandlerFailures()[0]
		assert.Same(t, nc, failure.Conn)
		assert.Equal(t, "orders.created", failure.Msg.Subject)
		assert.Equal(t, []byte("order-1"), failure.Msg.Data)
		assert.ErrorIs(t, failure.Err, errHandle)
	})

	t.Run("should keep serving the next message after a handler failed", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		handle := fixture.NewFakeHandlerFunc().FailWith(errors.New("boom"))
		handler := fixture.NewFakeHandler().WithHandleFunc("orders.created", "", handle.Handle)
		server := natsserver.NewServer(nc, handler).WithHooks(recorder.HandlerFailedHook())
		require.NoError(t, server.Init(ctx))

		require.NoError(t, nc.Publish("orders.created", []byte("order-1")))
		require.NoError(t, nc.Publish("orders.created", []byte("order-2")))

		handle.WaitForRequests(t, 2)
		recorder.WaitForEvents(t, 2)
		failures := recorder.HandlerFailures()
		assert.Equal(t, []byte("order-1"), failures[0].Msg.Data)
		assert.Equal(t, []byte("order-2"), failures[1].Msg.Data)
	})

	t.Run("should fire when a middleware wrapping the handler fails", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		errRejected := errors.New("the message is not signed")
		handle := fixture.NewFakeHandlerFunc()
		handler := fixture.NewFakeHandler().WithHandleFunc("orders.created", "", handle.Handle)
		server := natsserver.NewServer(nc, handler).WithHooks(recorder.HandlerFailedHook())
		server.Use(func(natsserver.HandlerFunc) natsserver.HandlerFunc {
			return func(*natsserver.Request, natsserver.PublishFunc) error { return errRejected }
		})
		require.NoError(t, server.Init(ctx))

		require.NoError(t, nc.Publish("orders.created", nil))

		assert.Equal(t, []string{"handler failed: orders.created: the message is not signed"},
			recorder.WaitForEvents(t, 1))
		assert.Zero(t, handle.CallCount(), "the middleware rejected the message before the handler")
	})

	t.Run("should not fire when the handler succeeds", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		handle := fixture.NewFakeHandlerFunc().ReplyWith([]byte("accepted"))
		handler := fixture.NewFakeHandler().WithHandleFunc("orders.created", "", handle.Handle)
		server := natsserver.NewServer(nc, handler).WithHooks(recorder.HandlerFailedHook())
		require.NoError(t, server.Init(ctx))

		reply, err := nc.Request("orders.created", []byte("order-1"), requestTimeout)

		require.NoError(t, err)
		assert.Equal(t, []byte("accepted"), reply.Data)
		assert.Never(t, func() bool { return len(recorder.Events()) > 0 }, quietPeriod, 10*time.Millisecond)
	})

	t.Run("should run every hook, in registration order, across several WithHooks calls", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		handler := fixture.NewFakeHandler().
			WithHandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().FailWith(errors.New("boom")).Handle)
		server := natsserver.NewServer(nc, handler).
			WithHooks(recorder.HooksLabeled("first")...).
			WithHooks(append(recorder.HooksLabeled("second"), recorder.HooksLabeled("third")...)...)
		require.NoError(t, server.Init(ctx))

		require.NoError(t, nc.Publish("orders.created", nil))

		assert.Equal(t, []string{
			"first: subscription opened: orders.created",
			"second: subscription opened: orders.created",
			"third: subscription opened: orders.created",
			"first: connection established",
			"second: connection established",
			"third: connection established",
			"first: handler failed: orders.created: boom",
			"second: handler failed: orders.created: boom",
			"third: handler failed: orders.created: boom",
		}, recorder.WaitForEvents(t, 9))
	})

	t.Run("should ignore a nil callback and still run the other hooks", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		handler := fixture.NewFakeHandler().
			WithHandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().FailWith(errors.New("boom")).Handle)
		server := natsserver.NewServer(nc, handler).
			WithHooks(natsserver.OnHandlerFailed(nil)).
			WithHooks(recorder.HandlerFailedHook()).
			WithHooks(natsserver.OnHandlerFailed(nil))
		require.NoError(t, server.Init(ctx))

		require.NoError(t, nc.Publish("orders.created", []byte("order-1")))
		require.NoError(t, nc.Publish("orders.created", []byte("order-2")))

		assert.Equal(t, []string{
			"handler failed: orders.created: boom",
			"handler failed: orders.created: boom",
		}, recorder.WaitForEvents(t, 2), "a nil callback must neither panic nor stop the dispatch")
	})
}

func TestOnConnectionEstablished(t *testing.T) {
	ctx := context.Background()

	t.Run("should fire once, with the connection, when Init succeeds", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		handler := fixture.NewFakeHandler().WithHandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().Handle)
		server := natsserver.NewServer(nc, handler).WithHooks(recorder.ConnectionEstablishedHook())

		require.NoError(t, server.Init(ctx))

		// The hook runs on the goroutine calling Init, so it already ran when Init returns.
		assert.Equal(t, []string{"connection established"}, recorder.Events())
		assert.Equal(t, []*nats.Conn{nc}, recorder.Connections())
	})

	t.Run("should run after the server released its lock, so that the hook may use the server", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc)
		// WithHooks takes the server mutex: the hook would deadlock if it ran while Init still held it.
		server.WithHooks(
			natsserver.OnConnectionEstablished(func(*nats.Conn) { server.WithHooks() }),
			recorder.ConnectionEstablishedHook(),
		)

		initialized := make(chan error, 1)
		go func() { initialized <- server.Init(ctx) }()

		require.Eventually(t, func() bool { return len(initialized) == 1 }, requestTimeout, 10*time.Millisecond,
			"Init never returned: the hook ran while the server held its lock")
		require.NoError(t, <-initialized)
		assert.Equal(t, []string{"connection established"}, recorder.Events())
	})

	t.Run("should not fire when Init fails", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		handler := fixture.NewFakeHandler().WithHandleFunc("orders created", "", fixture.NewFakeHandlerFunc().Handle)
		server := natsserver.NewServer(nc, handler).WithHooks(recorder.ConnectionEstablishedHook())

		err := server.Init(ctx)

		require.ErrorIs(t, err, nats.ErrBadSubject)
		assert.Empty(t, recorder.Events())
	})

	t.Run("should not fire when the connection is reconnecting at start, which is reported restored instead",
		func(t *testing.T) {
			natsServer := fixture.RunEmbeddedServer(t)
			nc := fixture.ConnectTo(t, natsServer, fixture.ReconnectQuickly())
			port := fixture.StopEmbeddedServer(t, natsServer)
			require.Eventually(t, nc.IsReconnecting, requestTimeout, 10*time.Millisecond)
			recorder := fixture.NewHookRecorder()
			handler := fixture.NewFakeHandler().
				WithHandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().Handle)
			server := natsserver.NewServer(nc, handler).WithHooks(recorder.ConnectionHooks()...)

			require.NoError(t, server.Init(ctx))
			assert.Empty(t, recorder.Events(), "a connection that is not connected is not established")

			fixture.RunEmbeddedServerOnPort(t, port)

			assert.Equal(t, []string{"connection restored"}, recorder.WaitForEvents(t, 1))
			assert.Equal(t, []*nats.Conn{nc}, recorder.Connections())
		})
}

func TestOnConnectionLost(t *testing.T) {
	ctx := context.Background()

	t.Run("should fire with the connection and the client error when the NATS server stops", func(t *testing.T) {
		natsServer := fixture.RunEmbeddedServer(t)
		nc := fixture.ConnectTo(t, natsServer, fixture.ReconnectQuickly())
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc).WithHooks(recorder.ConnectionLostHook())
		require.NoError(t, server.Init(ctx))

		fixture.StopEmbeddedServer(t, natsServer)

		events := recorder.WaitForEvents(t, 1)
		assert.True(t, strings.HasPrefix(events[0], "connection lost: "), "unexpected event %q", events[0])
		assert.Equal(t, []*nats.Conn{nc}, recorder.Connections())
	})

	t.Run("should fire each time the connection is lost", func(t *testing.T) {
		natsServer := fixture.RunEmbeddedServer(t)
		nc := fixture.ConnectTo(t, natsServer, fixture.ReconnectQuickly())
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc).WithHooks(recorder.ConnectionLostHook(), recorder.ConnectionRestoredHook())
		require.NoError(t, server.Init(ctx))

		port := fixture.StopEmbeddedServer(t, natsServer)
		recorder.WaitForEvents(t, 1)
		natsServer = fixture.RunEmbeddedServerOnPort(t, port)
		recorder.WaitForEvents(t, 2)
		fixture.StopEmbeddedServer(t, natsServer)

		events := recorder.WaitForEvents(t, 3)
		assert.True(t, strings.HasPrefix(events[0], "connection lost: "), "unexpected event %q", events[0])
		assert.Equal(t, "connection restored", events[1])
		assert.True(t, strings.HasPrefix(events[2], "connection lost: "), "unexpected event %q", events[2])
	})
}

func TestOnConnectionRestored(t *testing.T) {
	ctx := context.Background()

	t.Run("should fire with the connection when the NATS server comes back on the same port", func(t *testing.T) {
		natsServer := fixture.RunEmbeddedServer(t)
		nc := fixture.ConnectTo(t, natsServer, fixture.ReconnectQuickly())
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc).WithHooks(recorder.ConnectionRestoredHook())
		require.NoError(t, server.Init(ctx))
		port := fixture.StopEmbeddedServer(t, natsServer)
		require.Eventually(t, nc.IsReconnecting, requestTimeout, 10*time.Millisecond)

		fixture.RunEmbeddedServerOnPort(t, port)

		assert.Equal(t, []string{"connection restored"}, recorder.WaitForEvents(t, 1))
		assert.Equal(t, []*nats.Conn{nc}, recorder.Connections())
	})

	t.Run("should report a later connection as restored, never as established again", func(t *testing.T) {
		natsServer := fixture.RunEmbeddedServer(t)
		nc := fixture.ConnectTo(t, natsServer, fixture.ReconnectQuickly())
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc).WithHooks(recorder.Hooks()...)
		require.NoError(t, server.Init(ctx))

		port := fixture.StopEmbeddedServer(t, natsServer)
		recorder.WaitForEvents(t, 2)
		fixture.RunEmbeddedServerOnPort(t, port)

		events := recorder.WaitForEvents(t, 3)
		require.Len(t, events, 3)
		assert.Equal(t, "connection established", events[0])
		assert.True(t, strings.HasPrefix(events[1], "connection lost: "), "unexpected event %q", events[1])
		assert.Equal(t, "connection restored", events[2])
	})
}

func TestOnConnectionClosed(t *testing.T) {
	ctx := context.Background()

	t.Run("should fire with the connection when the caller closes it while the server runs", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc).WithHooks(recorder.ConnectionClosedHook())
		require.NoError(t, server.Init(ctx))

		nc.Close()

		assert.Equal(t, []string{"connection closed"}, recorder.WaitForEvents(t, 1))
		assert.Equal(t, []*nats.Conn{nc}, recorder.Connections())
	})

	t.Run("should fire when the client gives up reconnecting", func(t *testing.T) {
		natsServer := fixture.RunEmbeddedServer(t)
		nc := fixture.ConnectTo(t, natsServer, nats.NoReconnect())
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc).WithHooks(recorder.ConnectionClosedHook())
		require.NoError(t, server.Init(ctx))

		fixture.StopEmbeddedServer(t, natsServer)

		assert.Equal(t, []string{"connection closed"}, recorder.WaitForEvents(t, 1))
		assert.True(t, nc.IsClosed())
	})

	t.Run("should not fire when the connection is closed after Shutdown", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc).WithHooks(recorder.ConnectionClosedHook())
		require.NoError(t, server.Init(ctx))
		require.NoError(t, server.Shutdown(ctx))

		nc.Close()

		assert.Never(t, func() bool { return len(recorder.Events()) > 0 }, quietPeriod, 10*time.Millisecond)
	})

	t.Run("should run every connection hook in registration order and ignore a nil callback", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc).
			WithHooks(
				natsserver.OnConnectionEstablished(nil),
				natsserver.OnConnectionLost(nil),
				natsserver.OnConnectionRestored(nil),
				natsserver.OnConnectionClosed(nil),
			).
			WithHooks(recorder.HooksLabeled("first")...).
			WithHooks(append([]natsserver.Hook{natsserver.OnConnectionClosed(nil)}, recorder.HooksLabeled("second")...)...)
		require.NoError(t, server.Init(ctx))

		nc.Close()

		assert.Equal(t, []string{
			"first: connection established",
			"second: connection established",
			"first: connection lost: <nil>",
			"second: connection lost: <nil>",
			"first: connection closed",
			"second: connection closed",
		}, recorder.WaitForEvents(t, 6))
	})
}

// ordersHandler returns a Handler registering a handler function on orders.created in the queue group orders,
// then on orders.canceled without queue group.
func ordersHandler() *fixture.FakeHandler {
	handle := fixture.NewFakeHandlerFunc().Handle
	return fixture.NewFakeHandler().
		WithHandleFunc("orders.created", "orders", handle).
		WithHandleFunc("orders.canceled", "", handle)
}

func TestOnSubscriptionOpened(t *testing.T) {
	ctx := context.Background()

	t.Run("should fire once per registration, in registration order, during Init", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc, ordersHandler()).WithHooks(recorder.SubscriptionOpenedHook())

		require.NoError(t, server.Init(ctx))

		assert.Equal(t, []string{
			"subscription opened: orders.created",
			"subscription opened: orders.canceled",
		}, recorder.Events())
		subscriptions := recorder.Subscriptions()
		require.Len(t, subscriptions, 2)
		assert.Equal(t, "orders.created", subscriptions[0].Subject)
		assert.Equal(t, "orders", subscriptions[0].Queue)
		assert.Equal(t, "orders.canceled", subscriptions[1].Subject)
		assert.Empty(t, subscriptions[1].Queue)
		assert.True(t, subscriptions[0].IsValid(), "the hook receives the live subscription of the server")
	})

	t.Run("should report each subscription already opened as closed before Init returns the subscribe error",
		func(t *testing.T) {
			nc := fixture.ConnectToEmbeddedServer(t)
			recorder := fixture.NewHookRecorder()
			handle := fixture.NewFakeHandlerFunc().Handle
			handler := fixture.NewFakeHandler().
				WithHandleFunc("orders.created", "", handle).
				WithHandleFunc("orders canceled", "", handle)
			server := natsserver.NewServer(nc, handler).
				WithHooks(recorder.SubscriptionOpenedHook(), recorder.SubscriptionClosedHook())

			err := server.Init(ctx)

			require.ErrorIs(t, err, nats.ErrBadSubject)
			require.ErrorContains(t, err, `failed to subscribe to subject "orders canceled"`)
			// Init waits for the closed hooks, so they already ran when it returns.
			assert.Equal(t, []string{
				"subscription opened: orders.created",
				"subscription closed: orders.created",
			}, recorder.Events())
			assert.Equal(t, 0, nc.NumSubscriptions())
		})

	t.Run("should run every subscription hook in registration order and ignore a nil callback", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		handler := fixture.NewFakeHandler().WithHandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().Handle)
		server := natsserver.NewServer(nc, handler).
			WithHooks(
				natsserver.OnSubscriptionOpened(nil),
				natsserver.OnSubscriptionStatusChanged(nil),
				natsserver.OnSubscriptionClosed(nil),
			).
			WithHooks(recorder.HooksLabeled("first")...).
			WithHooks(append([]natsserver.Hook{natsserver.OnSubscriptionClosed(nil)}, recorder.HooksLabeled("second")...)...)
		require.NoError(t, server.Init(ctx))

		require.NoError(t, server.Shutdown(ctx))

		assert.Equal(t, []string{
			"first: subscription opened: orders.created",
			"second: subscription opened: orders.created",
		}, recorder.EventsContaining("subscription opened"))
		// The status and the closed hooks run on two goroutines, so their order is checked event by event.
		assert.Equal(t, []string{
			"first: subscription status changed: orders.created: Closed",
			"second: subscription status changed: orders.created: Closed",
		}, recorder.EventsContaining("subscription status changed"))
		assert.Equal(t, []string{
			"first: subscription closed: orders.created",
			"second: subscription closed: orders.created",
		}, recorder.EventsContaining("subscription closed"))
	})
}

func TestOnSubscriptionStatusChanged(t *testing.T) {
	ctx := context.Background()

	t.Run("should report Closed at Shutdown, and never the initial Active", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc, ordersHandler()).WithHooks(recorder.SubscriptionStatusChangedHook())
		require.NoError(t, server.Init(ctx))
		require.NoError(t, nc.Publish("orders.created", nil))
		require.NoError(t, nc.Flush())

		require.NoError(t, server.Shutdown(ctx))

		// Shutdown waits for the subscription hooks, so they already ran when it returns.
		assert.ElementsMatch(t, []string{
			"subscription status changed: orders.created: Closed",
			"subscription status changed: orders.canceled: Closed",
		}, recorder.Events())
	})

	t.Run("should report SlowConsumer when messages pile up faster than the handler takes them", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		handle := fixture.NewFakeHandlerFunc().BlockUntil(release)
		handler := fixture.NewFakeHandler().WithHandleFunc("orders.created", "", handle.Handle)
		server := natsserver.NewServer(nc, handler).WithHooks(
			// A subscription holding a single pending message becomes a slow consumer on the second one.
			natsserver.OnSubscriptionOpened(func(_ *nats.Conn, sub *nats.Subscription) {
				assert.NoError(t, sub.SetPendingLimits(1, -1))
			}),
			recorder.SubscriptionStatusChangedHook(),
		)
		require.NoError(t, server.Init(ctx))
		require.NoError(t, nc.Publish("orders.created", []byte("order-1")))
		handle.WaitForRequests(t, 1) // the handler holds the first message

		require.NoError(t, nc.Publish("orders.created", []byte("order-2")))
		require.NoError(t, nc.Publish("orders.created", []byte("order-3")))
		require.NoError(t, nc.Flush())

		assert.Equal(t, []string{"subscription status changed: orders.created: SlowConsumer"},
			recorder.WaitForEvents(t, 1))
	})

	t.Run("should report Closed and let Shutdown return when the connection closes while the server runs",
		func(t *testing.T) {
			nc := fixture.ConnectToEmbeddedServer(t)
			recorder := fixture.NewHookRecorder()
			server := natsserver.NewServer(nc, ordersHandler()).WithHooks(recorder.SubscriptionStatusChangedHook())
			require.NoError(t, server.Init(ctx))

			nc.Close()

			// The client reports no status when the connection closes: the server reports Closed itself.
			require.Eventually(t, func() bool { return len(recorder.Events()) == 2 }, requestTimeout,
				10*time.Millisecond, "the closed subscriptions were never reported Closed")
			assert.ElementsMatch(t, []string{
				"subscription status changed: orders.created: Closed",
				"subscription status changed: orders.canceled: Closed",
			}, recorder.Events())
			shutdownCtx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			err := server.Shutdown(shutdownCtx) // the unsubscribe error of the closed connection is expected
			require.NotErrorIs(t, err, context.DeadlineExceeded)
			assert.NoError(t, shutdownCtx.Err(), "Shutdown must not wait for a status goroutine left running")
		})

	t.Run("should make Shutdown wait for the status hooks until its context expires", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		release, _ := releaseOnce(t)
		server := natsserver.NewServer(nc, ordersHandler()).WithHooks(
			// The hook holds on until the test ends, so Shutdown can only stop waiting on its context.
			natsserver.OnSubscriptionStatusChanged(func(*nats.Conn, *nats.Subscription, nats.SubStatus) { <-release }),
		)
		require.NoError(t, server.Init(ctx))
		// A context that is still live when Shutdown starts waiting, so that a Shutdown that does not wait
		// returns no error instead of racing with an expired context.
		shutdownCtx, cancel := context.WithTimeout(ctx, quietPeriod)
		defer cancel()

		err := server.Shutdown(shutdownCtx)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.ErrorContains(t, err, "failed to wait for the NATS hooks")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})
}

// releaseOnce returns a release channel for hooks that block, and the function closing it, also called when the test
// ends so that no hook stays blocked.
func releaseOnce(t *testing.T) (<-chan struct{}, func()) {
	t.Helper()
	release := make(chan struct{})
	var once sync.Once
	closeRelease := func() { once.Do(func() { close(release) }) }
	t.Cleanup(closeRelease)
	return release, closeRelease
}

func TestOnSubscriptionClosed(t *testing.T) {
	ctx := context.Background()

	t.Run("should fire once per subscription, with the subscription, before Shutdown returns", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc, ordersHandler()).WithHooks(recorder.SubscriptionClosedHook())
		require.NoError(t, server.Init(ctx))
		require.Empty(t, recorder.Events(), "no subscription is closed while the server runs")

		require.NoError(t, server.Shutdown(ctx))

		// Each subscription closes on its own goroutine, in no set order.
		assert.ElementsMatch(t, []string{
			"subscription closed: orders.created",
			"subscription closed: orders.canceled",
		}, recorder.Events())
		subscriptions := recorder.Subscriptions()
		require.Len(t, subscriptions, 2)
		assert.False(t, subscriptions[0].IsValid())
		assert.False(t, subscriptions[1].IsValid())
	})

	t.Run("should fire once per subscription when the connection is closed while the server runs", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc, ordersHandler()).WithHooks(recorder.SubscriptionClosedHook())
		require.NoError(t, server.Init(ctx))

		nc.Close()

		assert.ElementsMatch(t, []string{
			"subscription closed: orders.created",
			"subscription closed: orders.canceled",
		}, recorder.WaitForEvents(t, 2))
	})

	t.Run("should make a failed Init wait for the closed hooks of the subscriptions already opened",
		func(t *testing.T) {
			nc := fixture.ConnectToEmbeddedServer(t)
			recorder := fixture.NewHookRecorder()
			release, closeRelease := releaseOnce(t)
			handle := fixture.NewFakeHandlerFunc().Handle
			handler := fixture.NewFakeHandler().
				WithHandleFunc("orders.created", "", handle).
				WithHandleFunc("orders canceled", "", handle)
			server := natsserver.NewServer(nc, handler).WithHooks(
				natsserver.OnSubscriptionClosed(func(*nats.Conn, *nats.Subscription) { <-release }),
				recorder.SubscriptionClosedHook(),
			)
			initialized := make(chan error, 1)
			go func() { initialized <- server.Init(ctx) }()

			assert.Never(t, func() bool { return len(initialized) == 1 }, quietPeriod, 10*time.Millisecond,
				"Init returned before the closed hooks ran")
			closeRelease()

			require.Eventually(t, func() bool { return len(initialized) == 1 }, requestTimeout, 10*time.Millisecond)
			require.ErrorIs(t, <-initialized, nats.ErrBadSubject)
			assert.Equal(t, []string{"subscription closed: orders.created"}, recorder.Events())
		})

	t.Run("should fail Shutdown and report StatusFailed when its context expires before the hooks ran",
		func(t *testing.T) {
			nc := fixture.ConnectToEmbeddedServer(t)
			release := make(chan struct{})
			t.Cleanup(func() { close(release) })
			server := natsserver.NewServer(nc, ordersHandler()).WithHooks(
				// The hook holds on until the test ends, so Shutdown can only stop waiting on its context.
				natsserver.OnSubscriptionClosed(func(*nats.Conn, *nats.Subscription) { <-release }),
			)
			require.NoError(t, server.Init(ctx))
			expiredCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
			defer cancel()

			err := server.Shutdown(expiredCtx)

			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.ErrorContains(t, err, "failed to wait for the NATS hooks")
			assert.Equal(t, resource.StatusFailed, server.Status().Get())
		})
}
