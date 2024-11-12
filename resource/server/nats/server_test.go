package nats_test

import (
	"context"
	"errors"
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

const requestTimeout = 2 * time.Second

func TestServer(t *testing.T) {
	ctx := context.Background()

	t.Run("should report StatusInitializing before Init", func(t *testing.T) {
		server := natsserver.NewServer(fixture.ConnectToEmbeddedServer(t))

		assert.Equal(t, resource.StatusInitializing, server.Status().Get())
	})

	t.Run("should use the default name", func(t *testing.T) {
		server := natsserver.NewServer(fixture.ConnectToEmbeddedServer(t))

		assert.Equal(t, natsserver.DefaultName, server.Name())
		assert.Equal(t, "nats-server", natsserver.DefaultName)
	})

	t.Run("should use the name set with WithName", func(t *testing.T) {
		server := natsserver.NewServer(fixture.ConnectToEmbeddedServer(t)).WithName("orders-messages")

		assert.Equal(t, "orders-messages", server.Name())
	})

	t.Run("should subscribe the registered handlers and report StatusHealthy", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		handler := fixture.NewFakeHandlerFunc()
		require.NoError(t, server.HandleFunc("orders.created", "", handler.Handle))

		err := server.Init(ctx)

		require.NoError(t, err)
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
		require.NoError(t, nc.Publish("orders.created", []byte("order")))
		requests := handler.WaitForRequests(t, 1)
		assert.Equal(t, []byte("order"), requests[0].Msg.Data)
	})

	t.Run("should register the handlers given to the constructor when Init runs", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		handler := fixture.NewFakeHandler().
			WithHandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().Handle)
		server := natsserver.NewServer(nc, handler)
		require.False(t, handler.WasRegistered(), "the handlers must be registered by Init, not by the constructor")

		err := server.Init(ctx)

		require.NoError(t, err)
		assert.True(t, handler.WasRegistered())
		assert.Equal(t, 1, nc.NumSubscriptions())
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
	})

	t.Run("fails to init when a handler cannot register", func(t *testing.T) {
		errRegister := errors.New("registration failed")
		handler := fixture.NewFakeHandler().FailRegisterWith(errRegister)
		server := natsserver.NewServer(fixture.ConnectToEmbeddedServer(t), handler)

		err := server.Init(ctx)

		require.ErrorIs(t, err, errRegister)
		require.ErrorContains(t, err, "failed to register a NATS handler")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init when the NATS connection is nil", func(t *testing.T) {
		server := natsserver.NewServer(nil)

		err := server.Init(ctx)

		require.ErrorIs(t, err, natsserver.ErrNilNATSConnection)
		require.ErrorContains(t, err, "failed to init the NATS server")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init when the connection cannot subscribe", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		require.NoError(t, server.HandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().Handle))
		nc.Close()

		err := server.Init(ctx)

		require.ErrorIs(t, err, nats.ErrConnectionClosed)
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init twice, leaving the running server untouched", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		handler := fixture.NewFakeHandler().
			WithHandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().Handle)
		server := natsserver.NewServer(nc, handler)
		require.NoError(t, server.Init(ctx))

		err := server.Init(ctx)

		require.ErrorIs(t, err, natsserver.ErrServerAlreadyStarted)
		require.ErrorContains(t, err, "failed to init the NATS server")
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
		assert.Equal(t, 1, nc.NumSubscriptions())
	})

	t.Run("should report StatusClosed after a graceful shutdown", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		require.NoError(t, server.HandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().Handle))
		require.NoError(t, server.Init(ctx))

		err := server.Shutdown(ctx)

		require.NoError(t, err)
		assert.Equal(t, resource.StatusClosed, server.Status().Get())
		assert.Equal(t, 0, nc.NumSubscriptions())
	})

	t.Run("fails to init again after a shutdown, staying closed", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		require.NoError(t, server.HandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().Handle))
		require.NoError(t, server.Init(ctx))
		require.NoError(t, server.Shutdown(ctx))

		err := server.Init(ctx)

		require.ErrorIs(t, err, natsserver.ErrServerAlreadyStarted)
		assert.Equal(t, resource.StatusClosed, server.Status().Get())
		assert.Equal(t, 0, nc.NumSubscriptions())
	})

	t.Run("fails to shut down and reports StatusFailed when a subscription cannot be canceled", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		require.NoError(t, server.HandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().Handle))
		require.NoError(t, server.Init(ctx))
		nc.Close()

		err := server.Shutdown(ctx)

		require.ErrorIs(t, err, nats.ErrConnectionClosed)
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("should report StatusClosed when Init was never called", func(t *testing.T) {
		server := natsserver.NewServer(fixture.ConnectToEmbeddedServer(t))

		err := server.Shutdown(ctx)

		require.NoError(t, err)
		assert.Equal(t, resource.StatusClosed, server.Status().Get())
	})

	t.Run("should be manageable by a resource.List", func(t *testing.T) {
		server := natsserver.NewServer(fixture.ConnectToEmbeddedServer(t))
		require.NoError(t, server.HandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().Handle))

		list := resource.NewList()
		require.NoError(t, list.Register(server))
		require.NoError(t, list.Init(ctx))
		assert.Equal(t, resource.StatusHealthy, list.Status().Get())

		require.NoError(t, list.Shutdown(ctx))
		assert.Equal(t, resource.StatusClosed, list.Status().Get())
	})

	t.Run("fails to register a handler with an empty subject", func(t *testing.T) {
		server := natsserver.NewServer(fixture.ConnectToEmbeddedServer(t))

		err := server.HandleFunc("", "", fixture.NewFakeHandlerFunc().Handle)

		require.ErrorIs(t, err, natsserver.ErrEmptySubject)
		require.ErrorContains(t, err, "failed to register a handler")
	})

	t.Run("fails to register a handler once the server is started", func(t *testing.T) {
		server := natsserver.NewServer(fixture.ConnectToEmbeddedServer(t))
		require.NoError(t, server.ListenAndServe())

		err := server.HandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().Handle)

		require.ErrorIs(t, err, natsserver.ErrServerAlreadyStarted)
		require.ErrorContains(t, err, "failed to register a handler")
	})

	t.Run("fails to start when a subject is refused by the connection", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		require.NoError(t, server.HandleFunc("orders created", "", fixture.NewFakeHandlerFunc().Handle))

		err := server.ListenAndServe()

		require.ErrorIs(t, err, nats.ErrBadSubject)
		require.ErrorContains(t, err, `failed to subscribe to subject "orders created"`)
	})

	t.Run("fails to start twice, leaving a single subscription per handler", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		require.NoError(t, server.HandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().Handle))
		require.NoError(t, server.ListenAndServe())

		err := server.ListenAndServe()

		require.ErrorIs(t, err, natsserver.ErrServerAlreadyStarted)
		require.ErrorContains(t, err, "failed to start the NATS server")
		assert.Equal(t, 1, nc.NumSubscriptions())
	})

	t.Run("should hold no subscription and stay startable after a failed start", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		handle := fixture.NewFakeHandlerFunc().Handle
		require.NoError(t, server.HandleFunc("orders.created", "", handle))
		require.NoError(t, server.HandleFunc("orders created", "", handle))
		require.ErrorIs(t, server.ListenAndServe(), nats.ErrBadSubject)

		assert.Equal(t, 0, nc.NumSubscriptions(), "the subscriptions made before the failure must be canceled")
		require.NoError(t, server.HandleFunc("orders.deleted", "", handle),
			"a server that failed to start is not started")
		err := server.ListenAndServe()
		assert.NotErrorIs(t, err, natsserver.ErrServerAlreadyStarted)
	})

	t.Run("should cancel the context of the requests when the server is closed", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		handler := fixture.NewFakeHandlerFunc()
		require.NoError(t, server.HandleFunc("orders.created", "", handler.Handle))
		require.NoError(t, server.ListenAndServe())
		require.NoError(t, nc.Publish("orders.created", nil))
		requestCtx := handler.WaitForRequests(t, 1)[0].Context
		require.NoError(t, requestCtx.Err(), "the context must be live while the server runs")

		require.NoError(t, server.Close())

		assert.ErrorIs(t, requestCtx.Err(), context.Canceled)
	})

	t.Run("should cancel the context of the requests when the server is shut down", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		handler := fixture.NewFakeHandlerFunc()
		server := natsserver.NewServer(nc,
			fixture.NewFakeHandler().WithHandleFunc("orders.created", "", handler.Handle))
		require.NoError(t, server.Init(ctx))
		require.NoError(t, nc.Publish("orders.created", nil))
		requestCtx := handler.WaitForRequests(t, 1)[0].Context

		require.NoError(t, server.Shutdown(ctx))

		assert.ErrorIs(t, requestCtx.Err(), context.Canceled)
	})

	t.Run("should let the error handler change while messages are dispatched", func(t *testing.T) {
		const messageCount = 100
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		handler := fixture.NewFakeHandlerFunc().FailWith(errors.New("boom"))
		require.NoError(t, server.HandleFunc("orders.created", "", handler.Handle))
		require.NoError(t, server.ListenAndServe())

		// The race detector fails the test when the dispatch reads the error handler without synchronization.
		var group sync.WaitGroup
		group.Add(2)
		go func() {
			defer group.Done()
			for i := 0; i < messageCount; i++ {
				server.OnError(func(error) {})
			}
		}()
		go func() {
			defer group.Done()
			for i := 0; i < messageCount; i++ {
				assert.NoError(t, nc.Publish("orders.created", nil))
			}
		}()
		group.Wait()

		handler.WaitForRequests(t, messageCount)
	})

	t.Run("should unsubscribe every subscription on Close", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		handle := fixture.NewFakeHandlerFunc().Handle
		require.NoError(t, server.HandleFunc("orders.created", "", handle))
		require.NoError(t, server.HandleFunc("orders.deleted", "", handle))
		require.NoError(t, server.ListenAndServe())
		require.Equal(t, 2, nc.NumSubscriptions())

		require.NoError(t, server.Close())

		assert.Equal(t, 0, nc.NumSubscriptions())
	})

	t.Run("fails to close with the first error when subscriptions cannot be canceled", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		handle := fixture.NewFakeHandlerFunc().Handle
		require.NoError(t, server.HandleFunc("orders.created", "", handle))
		require.NoError(t, server.HandleFunc("orders.deleted", "", handle))
		require.NoError(t, server.ListenAndServe())
		nc.Close()

		err := server.Close()

		require.ErrorIs(t, err, nats.ErrConnectionClosed)
		require.ErrorContains(t, err, "failed to unsubscribe")
	})
}
