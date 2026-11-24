package fixture_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	natsserver "github.com/grandper/go-athanor/resource/server/nats"
	"github.com/grandper/go-athanor/resource/server/nats/fixture"
)

// serveFailingHandler starts a server on nc whose handler on orders.created fails with err, hooked with hooks.
// The server is shut down when the test ends.
func serveFailingHandler(t *testing.T, nc *nats.Conn, err error, hooks ...natsserver.Hook) *natsserver.Server {
	t.Helper()
	handler := fixture.NewFakeHandler().
		WithHandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().FailWith(err).Handle)
	server := natsserver.NewServer(nc, handler).WithHooks(hooks...)
	require.NoError(t, server.Init(context.Background()))
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })
	return server
}

// publishOrder publishes data on orders.created.
func publishOrder(t *testing.T, nc *nats.Conn, data string) {
	t.Helper()
	require.NoError(t, nc.Publish("orders.created", []byte(data)))
}

func TestHookRecorder(t *testing.T) {
	t.Run("should record nothing before the server reports an event", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()

		assert.Empty(t, recorder.Events())
		assert.Empty(t, recorder.Connections())
		assert.Empty(t, recorder.Subscriptions())
		assert.Empty(t, recorder.HandlerFailures())
	})

	t.Run("should return one hook per lifecycle event", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()

		assert.Len(t, recorder.Hooks(), 8)
		assert.Len(t, recorder.HooksLabeled("first"), 8)
		assert.Len(t, recorder.ConnectionHooks(), 4)
		assert.Len(t, recorder.SubscriptionHooks(), 3)
	})

	t.Run("should record the connection established at start, with the connection", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		nc := fixture.ConnectToEmbeddedServer(t)

		serveFailingHandler(t, nc, errors.New("boom"), recorder.Hooks()...)

		assert.Equal(t, []string{"subscription opened: orders.created", "connection established"}, recorder.Events())
		assert.Equal(t, []*nats.Conn{nc}, recorder.Connections())
	})

	t.Run("should record the connection lost, then restored, when the NATS server restarts", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		natsServer := fixture.RunEmbeddedServer(t)
		nc := fixture.ConnectTo(t, natsServer, fixture.ReconnectQuickly())
		serveFailingHandler(t, nc, errors.New("boom"), recorder.ConnectionHooks()...)

		port := fixture.StopEmbeddedServer(t, natsServer)
		lost := recorder.WaitForEvents(t, 2)[1]
		fixture.RunEmbeddedServerOnPort(t, port)

		assert.True(t, strings.HasPrefix(lost, "connection lost: "), "unexpected event %q", lost)
		assert.Equal(t, "connection restored", recorder.WaitForEvents(t, 3)[2])
		assert.Equal(t, []*nats.Conn{nc, nc, nc}, recorder.Connections())
	})

	t.Run("should record the connection lost without error, then closed, when the connection is closed",
		func(t *testing.T) {
			recorder := fixture.NewHookRecorder()
			nc := fixture.ConnectToEmbeddedServer(t)
			serveFailingHandler(t, nc, errors.New("boom"), recorder.ConnectionHooks()...)

			nc.Close()

			assert.Equal(t, []string{
				"connection established",
				"connection lost: <nil>",
				"connection closed",
			}, recorder.WaitForEvents(t, 3))
		})

	t.Run("should record a handler failure with its subject and error", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		nc := fixture.ConnectToEmbeddedServer(t)
		serveFailingHandler(t, nc, errors.New("the order has no line"), recorder.Hooks()...)

		publishOrder(t, nc, "order-1")

		assert.Equal(t, []string{
			"subscription opened: orders.created",
			"connection established",
			"handler failed: orders.created: the order has no line",
		}, recorder.WaitForEvents(t, 3))
	})

	t.Run("should record what the OnHandlerFailed hook received", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		errHandle := errors.New("boom")
		nc := fixture.ConnectToEmbeddedServer(t)
		serveFailingHandler(t, nc, errHandle, recorder.HandlerFailedHook())

		publishOrder(t, nc, "order-1")

		assert.Equal(t, []string{"handler failed: orders.created: boom"}, recorder.WaitForEvents(t, 1))
		failures := recorder.HandlerFailures()
		require.Len(t, failures, 1)
		assert.Same(t, nc, failures[0].Conn)
		assert.Equal(t, "orders.created", failures[0].Msg.Subject)
		assert.Equal(t, []byte("order-1"), failures[0].Msg.Data)
		assert.ErrorIs(t, failures[0].Err, errHandle)
	})

	t.Run("should record the events in the order they happened", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		nc := fixture.ConnectToEmbeddedServer(t)
		serveFailingHandler(t, nc, errors.New("boom"), recorder.HandlerFailedHook())

		publishOrder(t, nc, "order-1")
		publishOrder(t, nc, "order-2")

		recorder.WaitForEvents(t, 2)
		failures := recorder.HandlerFailures()
		require.Len(t, failures, 2)
		assert.Equal(t, []byte("order-1"), failures[0].Msg.Data)
		assert.Equal(t, []byte("order-2"), failures[1].Msg.Data)
	})

	t.Run("should record only the connection established with ConnectionEstablishedHook", func(t *testing.T) {
		recorder, callbacks := fixture.NewHookRecorder(), fixture.NewConnectCallbackRecorder()
		nc := fixture.ConnectToEmbeddedServer(t, callbacks.Options()...)
		serveFailingHandler(t, nc, errors.New("boom"), recorder.ConnectionEstablishedHook())

		nc.Close()

		callbacks.WaitForEvents(t, 2) // the closed callback runs last
		assert.Equal(t, []string{"connection established"}, recorder.Events())
	})

	t.Run("should record only the connection lost with ConnectionLostHook", func(t *testing.T) {
		recorder, callbacks := fixture.NewHookRecorder(), fixture.NewConnectCallbackRecorder()
		nc := fixture.ConnectToEmbeddedServer(t, callbacks.Options()...)
		serveFailingHandler(t, nc, errors.New("boom"), recorder.ConnectionLostHook())

		nc.Close()

		assert.Equal(t, []string{"closed"}, callbacks.WaitForEvents(t, 1)) // the closed callback runs last
		assert.Equal(t, []string{"connection lost: <nil>"}, recorder.Events())
	})

	t.Run("should record only the connection restored with ConnectionRestoredHook", func(t *testing.T) {
		recorder, callbacks := fixture.NewHookRecorder(), fixture.NewConnectCallbackRecorder()
		natsServer := fixture.RunEmbeddedServer(t)
		nc := fixture.ConnectTo(t, natsServer, append(callbacks.Options(), fixture.ReconnectQuickly())...)
		serveFailingHandler(t, nc, errors.New("boom"), recorder.ConnectionRestoredHook())

		port := fixture.StopEmbeddedServer(t, natsServer)
		callbacks.WaitForEvents(t, 1)
		fixture.RunEmbeddedServerOnPort(t, port)

		assert.Equal(t, []string{"connection restored"}, recorder.WaitForEvents(t, 1))
		assert.Equal(t, []*nats.Conn{nc}, recorder.Connections())
	})

	t.Run("should record only the connection closed with ConnectionClosedHook", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		nc := fixture.ConnectToEmbeddedServer(t)
		serveFailingHandler(t, nc, errors.New("boom"), recorder.ConnectionClosedHook())

		nc.Close()

		assert.Equal(t, []string{"connection closed"}, recorder.WaitForEvents(t, 1))
		assert.Equal(t, []*nats.Conn{nc}, recorder.Connections())
	})

	t.Run("should prefix the events of labeled hooks with their label", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		nc := fixture.ConnectToEmbeddedServer(t)
		serveFailingHandler(t, nc, errors.New("boom"),
			append(recorder.HooksLabeled("first"), recorder.HooksLabeled("second")...)...)

		publishOrder(t, nc, "order-1")

		assert.Equal(t, []string{
			"first: subscription opened: orders.created",
			"second: subscription opened: orders.created",
			"first: connection established",
			"second: connection established",
			"first: handler failed: orders.created: boom",
			"second: handler failed: orders.created: boom",
		}, recorder.WaitForEvents(t, 6))
	})

	t.Run("should return a snapshot of the events", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		serveFailingHandler(t, fixture.ConnectToEmbeddedServer(t), errors.New("boom"), recorder.Hooks()...)
		events := recorder.Events()

		events[0] = "changed"

		assert.Equal(t, []string{"subscription opened: orders.created", "connection established"}, recorder.Events())
	})

	t.Run("should record the subscription opened, with the subscription", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		nc := fixture.ConnectToEmbeddedServer(t)

		serveFailingHandler(t, nc, errors.New("boom"), recorder.SubscriptionOpenedHook())

		assert.Equal(t, []string{"subscription opened: orders.created"}, recorder.Events())
		subscriptions := recorder.Subscriptions()
		require.Len(t, subscriptions, 1)
		assert.Equal(t, "orders.created", subscriptions[0].Subject)
	})

	t.Run("should record the subscription status changed with the text of the status", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		server := serveFailingHandler(t, fixture.ConnectToEmbeddedServer(t), errors.New("boom"),
			recorder.SubscriptionStatusChangedHook())

		require.NoError(t, server.Shutdown(context.Background()))

		assert.Equal(t, []string{"subscription status changed: orders.created: Closed"}, recorder.Events())
		assert.Len(t, recorder.Subscriptions(), 1)
	})

	t.Run("should record the subscription closed", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		server := serveFailingHandler(t, fixture.ConnectToEmbeddedServer(t), errors.New("boom"),
			recorder.SubscriptionClosedHook())

		require.NoError(t, server.Shutdown(context.Background()))

		assert.Equal(t, []string{"subscription closed: orders.created"}, recorder.Events())
		assert.Len(t, recorder.Subscriptions(), 1)
	})

	t.Run("should record every subscription event with the subscription hooks", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		server := serveFailingHandler(t, fixture.ConnectToEmbeddedServer(t), errors.New("boom"),
			recorder.SubscriptionHooks()...)

		require.NoError(t, server.Shutdown(context.Background()))

		events := recorder.Events()
		require.Len(t, events, 3)
		assert.Equal(t, "subscription opened: orders.created", events[0])
		// The status and the closed hooks run on two goroutines of the client, in no set order.
		assert.ElementsMatch(t, []string{
			"subscription status changed: orders.created: Closed",
			"subscription closed: orders.created",
		}, events[1:])
	})

	t.Run("should return the events containing a text, in order", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		nc := fixture.ConnectToEmbeddedServer(t)
		serveFailingHandler(t, nc, errors.New("boom"), recorder.Hooks()...)
		publishOrder(t, nc, "order-1")
		publishOrder(t, nc, "order-2")
		recorder.WaitForEvents(t, 4)

		assert.Equal(t, []string{
			"handler failed: orders.created: boom",
			"handler failed: orders.created: boom",
		}, recorder.EventsContaining("handler failed"))
		assert.Equal(t, []string{"connection established"}, recorder.EventsContaining("connection"))
		assert.Empty(t, recorder.EventsContaining("restored"))
	})
}
