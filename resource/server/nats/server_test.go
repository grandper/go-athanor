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

	t.Run("should return the same server from WithHooks", func(t *testing.T) {
		server := natsserver.NewServer(fixture.ConnectToEmbeddedServer(t))

		assert.Same(t, server, server.WithHooks(fixture.NewHookRecorder().Hooks()...))
	})

	t.Run("should keep the hooks of every WithHooks call", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		first, second := fixture.NewHookRecorder(), fixture.NewHookRecorder()
		handler := fixture.NewFakeHandler().
			WithHandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().FailWith(errors.New("boom")).Handle)
		server := natsserver.NewServer(nc, handler).WithHooks(first.Hooks()...)
		server.WithHooks(second.Hooks()...)
		require.NoError(t, server.Init(ctx))

		require.NoError(t, nc.Publish("orders.created", nil))

		want := []string{
			"subscription opened: orders.created",
			"connection established",
			"handler failed: orders.created: boom",
		}
		assert.Equal(t, want, first.WaitForEvents(t, 3))
		assert.Equal(t, want, second.WaitForEvents(t, 3))
	})

	t.Run("should ignore the hooks added after Init", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		early, late := fixture.NewHookRecorder(), fixture.NewHookRecorder()
		handler := fixture.NewFakeHandler().
			WithHandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().FailWith(errors.New("boom")).Handle)
		server := natsserver.NewServer(nc, handler).WithHooks(early.Hooks()...)
		require.NoError(t, server.Init(ctx))

		server.WithHooks(late.Hooks()...)
		require.NoError(t, nc.Publish("orders.created", nil))
		require.NoError(t, nc.Publish("orders.created", nil))

		// The messages of a subscription are dispatched one after the other, so once the early hooks saw the
		// second message, a late hook would have finished with the first one.
		early.WaitForEvents(t, 4)
		assert.Empty(t, late.Events(), "a hook added after Init must be ignored")
	})

	t.Run("should leave the nats.Connect callbacks untouched when no hook watches the connection", func(t *testing.T) {
		callbacks := fixture.NewConnectCallbackRecorder()
		natsServer := fixture.RunEmbeddedServer(t)
		nc := fixture.ConnectTo(t, natsServer, append(callbacks.Options(), fixture.ReconnectQuickly())...)
		server := natsserver.NewServer(nc).WithHooks(fixture.NewHookRecorder().HandlerFailedHook())
		require.NoError(t, server.Init(ctx))

		port := fixture.StopEmbeddedServer(t, natsServer)
		callbacks.WaitForEvents(t, 1)
		fixture.RunEmbeddedServerOnPort(t, port)
		callbacks.WaitForEvents(t, 2)
		nc.Close()

		events := callbacks.WaitForEvents(t, 4)
		assert.True(t, strings.HasPrefix(events[0], "disconnected: "), "unexpected event %q", events[0])
		assert.Equal(t, []string{"reconnected", "disconnected: <nil>", "closed"}, events[1:])
	})

	t.Run("should replace the nats.Connect callback of each event a hook watches, and only those", func(t *testing.T) {
		callbacks := fixture.NewConnectCallbackRecorder()
		nc := fixture.ConnectToEmbeddedServer(t, callbacks.Options()...)
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc).WithHooks(recorder.ConnectionClosedHook())
		require.NoError(t, server.Init(ctx))

		nc.Close()

		assert.Equal(t, []string{"connection closed"}, recorder.WaitForEvents(t, 1))
		// The client calls the disconnect callback before the closed one, on the same goroutine.
		assert.Equal(t, []string{"disconnected: <nil>"}, callbacks.Events(),
			"the closed callback is replaced by the hook, while the disconnect callback is still called")
	})

	t.Run("should reset to nil the connection callbacks it installed when it shuts down", func(t *testing.T) {
		callbacks := fixture.NewConnectCallbackRecorder()
		nc := fixture.ConnectToEmbeddedServer(t, callbacks.Options()...)
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc).WithHooks(recorder.Hooks()...)
		require.NoError(t, server.Init(ctx))

		require.NoError(t, server.Shutdown(ctx))

		assert.Nil(t, nc.DisconnectErrHandler())
		assert.Nil(t, nc.ReconnectHandler())
		assert.Nil(t, nc.ClosedHandler())
	})

	t.Run("should keep the nats.Connect callbacks it did not replace after it shuts down", func(t *testing.T) {
		callbacks := fixture.NewConnectCallbackRecorder()
		nc := fixture.ConnectToEmbeddedServer(t, callbacks.Options()...)
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc).WithHooks(recorder.ConnectionClosedHook())
		require.NoError(t, server.Init(ctx))
		require.NoError(t, server.Shutdown(ctx))

		nc.Close()

		assert.Equal(t, []string{"disconnected: <nil>"}, callbacks.WaitForEvents(t, 1))
		assert.Nil(t, nc.ClosedHandler(), "the replaced closed callback is reset, not restored")
		assert.Never(t, func() bool { return len(recorder.Events()) > 0 || len(callbacks.Events()) > 1 },
			quietPeriod, 10*time.Millisecond)
	})

	t.Run("should leave the nats.Connect callbacks untouched when Init fails", func(t *testing.T) {
		callbacks := fixture.NewConnectCallbackRecorder()
		nc := fixture.ConnectToEmbeddedServer(t, callbacks.Options()...)
		recorder := fixture.NewHookRecorder()
		handler := fixture.NewFakeHandler().WithHandleFunc("orders created", "", fixture.NewFakeHandlerFunc().Handle)
		server := natsserver.NewServer(nc, handler).WithHooks(recorder.Hooks()...)
		require.ErrorIs(t, server.Init(ctx), nats.ErrBadSubject)

		nc.Close()

		assert.Equal(t, []string{"disconnected: <nil>", "closed"}, callbacks.WaitForEvents(t, 2))
		assert.Empty(t, recorder.Events(), "a failed start installs no connection hook")
	})

	t.Run("should leave the nats.Connect callbacks untouched when shut down without having started",
		func(t *testing.T) {
			callbacks := fixture.NewConnectCallbackRecorder()
			nc := fixture.ConnectToEmbeddedServer(t, callbacks.Options()...)
			recorder := fixture.NewHookRecorder()
			server := natsserver.NewServer(nc).WithHooks(recorder.Hooks()...)
			require.NoError(t, server.Shutdown(ctx))

			nc.Close()

			assert.Equal(t, []string{"disconnected: <nil>", "closed"}, callbacks.WaitForEvents(t, 2))
			assert.Empty(t, recorder.Events())
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

	t.Run("should reset its connection callbacks even when a subscription cannot be canceled", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		handler := fixture.NewFakeHandler().WithHandleFunc("orders.created", "", fixture.NewFakeHandlerFunc().Handle)
		server := natsserver.NewServer(nc, handler).WithHooks(recorder.Hooks()...)
		require.NoError(t, server.Init(ctx))
		nc.Close()
		// The closed connection closed the subscription: once its hook ran, every hook of the server is done.
		require.Eventually(t, func() bool { return len(recorder.EventsContaining("subscription closed")) == 1 },
			requestTimeout, 10*time.Millisecond)
		shutdownCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		defer cancel()

		err := server.Shutdown(shutdownCtx)

		require.ErrorIs(t, err, nats.ErrConnectionClosed, "the unsubscribe error is what Shutdown reports")
		require.NotErrorIs(t, err, context.DeadlineExceeded, "no hook is left to wait for")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
		assert.Nil(t, nc.DisconnectErrHandler())
		assert.Nil(t, nc.ReconnectHandler())
		assert.Nil(t, nc.ClosedHandler())
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

	t.Run("should let hooks be added while messages are dispatched", func(t *testing.T) {
		const messageCount = 100
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc).WithHooks(fixture.NewHookRecorder().Hooks()...)
		handler := fixture.NewFakeHandlerFunc().FailWith(errors.New("boom"))
		require.NoError(t, server.HandleFunc("orders.created", "", handler.Handle))
		require.NoError(t, server.ListenAndServe())

		// The race detector fails the test when the dispatch reads the hooks without synchronization.
		var group sync.WaitGroup
		group.Add(2)
		go func() {
			defer group.Done()
			for range messageCount {
				server.WithHooks(fixture.NewHookRecorder().Hooks()...)
			}
		}()
		go func() {
			defer group.Done()
			for range messageCount {
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
