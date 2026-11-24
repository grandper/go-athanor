package http_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpserver "github.com/grandper/go-athanor/resource/server/http"
	"github.com/grandper/go-athanor/resource/server/http/fixture"
)

// quietPeriod is how long a test waits to be confident that something does not happen.
const quietPeriod = 100 * time.Millisecond

// serveOnFreePort starts server, built from config, on a free port of the loopback interface, shuts it down when
// the test ends, and returns its base URL.
func serveOnFreePort(t *testing.T, config *httpserver.Config, server *httpserver.Server) string {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, fixture.InitOnFreePort(ctx, t, config, server.Init))
	t.Cleanup(func() { _ = server.Shutdown(ctx) })
	return "http://" + config.Address()
}

// newOrdersServer returns the configuration of a loopback server, and a server whose route /orders/42 panics with
// value while /orders answers 200 OK.
func newOrdersServer(value any) (*httpserver.Config, *httpserver.Server) {
	config := &httpserver.Config{BindInterface: "127.0.0.1"}
	server := httpserver.NewServer(config,
		fixture.NewPanickingHandler("/orders/42", value),
		fixture.NewFakeHandler().WithRoute("/orders", http.StatusOK),
	)
	return config, server
}

func TestOnHandlerPanicked(t *testing.T) {
	ctx := context.Background()

	t.Run("should answer 500, hand the hook the request and the recovered value, and keep serving",
		func(t *testing.T) {
			recorder := fixture.NewHookRecorder()
			config, server := newOrdersServer("the order has no line")
			baseURL := serveOnFreePort(t, config, server.WithHooks(recorder.HandlerPanickedHook()))

			response, err := fixture.GetWithoutKeepAlive(ctx, baseURL+"/orders/42")

			require.NoError(t, err)
			assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
			// The recovery calls the hooks before it answers, so they already ran.
			assert.Equal(t, []string{"handler panicked: GET /orders/42: the order has no line"}, recorder.Events())
			panics := recorder.Panics()
			require.Len(t, panics, 1)
			assert.Equal(t, http.MethodGet, panics[0].Request.Method)
			assert.Equal(t, "/orders/42", panics[0].Request.URL.Path)
			assert.Equal(t, "the order has no line", panics[0].Recovered)
			next, err := fixture.GetWithoutKeepAlive(ctx, baseURL+"/orders")
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, next.StatusCode)
		})

	t.Run("should hand the hook the error a handler panicked with", func(t *testing.T) {
		errPanic := errors.New("nil order")
		recorder := fixture.NewHookRecorder()
		config, server := newOrdersServer(errPanic)
		baseURL := serveOnFreePort(t, config, server.WithHooks(recorder.HandlerPanickedHook()))

		response, err := fixture.GetWithoutKeepAlive(ctx, baseURL+"/orders/42")

		require.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
		recovered, ok := recorder.Panics()[0].Recovered.(error)
		require.True(t, ok, "the recovered value must be the error itself")
		assert.ErrorIs(t, recovered, errPanic)
	})

	t.Run("should not call the hooks for a handler aborted with http.ErrAbortHandler", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		config, server := newOrdersServer(http.ErrAbortHandler)
		baseURL := serveOnFreePort(t, config, server.WithHooks(recorder.Hooks()...))

		_, err := fixture.GetWithoutKeepAlive(ctx, baseURL+"/orders/42")

		require.Error(t, err, "the client must see the aborted response")
		// net/http drops the connection after the hooks and its log would have run.
		assert.Empty(t, recorder.EventsContaining("handler panicked"))
		assert.Empty(t, recorder.EventsContaining("error logged"))
	})

	t.Run("should keep the behavior of net/http without an OnHandlerPanicked hook", func(t *testing.T) {
		config, server := newOrdersServer("the order has no line")
		baseURL := serveOnFreePort(t, config, server.WithHooks(fixture.NewHookRecorder().ErrorLoggedHook()))

		_, err := fixture.GetWithoutKeepAlive(ctx, baseURL+"/orders/42")

		require.Error(t, err, "net/http drops the connection of a panicking handler instead of answering")
		next, err := fixture.GetWithoutKeepAlive(ctx, baseURL+"/orders")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, next.StatusCode)
	})

	t.Run("should run every hook, in registration order, across several WithHooks calls", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		config, server := newOrdersServer("boom")
		server.
			WithHooks(recorder.HooksLabeled("first")...).
			WithHooks(append(recorder.HooksLabeled("second"), recorder.HooksLabeled("third")...)...)
		baseURL := serveOnFreePort(t, config, server)

		_, err := fixture.GetWithoutKeepAlive(ctx, baseURL+"/orders/42")

		require.NoError(t, err)
		assert.Equal(t, []string{
			"first: handler panicked: GET /orders/42: boom",
			"second: handler panicked: GET /orders/42: boom",
			"third: handler panicked: GET /orders/42: boom",
		}, recorder.EventsContaining("handler panicked"))
	})

	t.Run("should ignore a nil callback and still run the other hooks", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		config, server := newOrdersServer("boom")
		server.
			WithHooks(httpserver.OnHandlerPanicked(nil), httpserver.OnErrorLogged(nil)).
			WithHooks(recorder.Hooks()...).
			WithHooks(httpserver.OnHandlerPanicked(nil))
		baseURL := serveOnFreePort(t, config, server)

		response, err := fixture.GetWithoutKeepAlive(ctx, baseURL+"/orders/42")

		require.NoError(t, err, "a nil callback must neither panic nor stop the recovery")
		assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
		assert.Equal(t, []string{"handler panicked: GET /orders/42: boom"},
			recorder.EventsContaining("handler panicked"))
	})

	t.Run("should not install the recovery when every OnHandlerPanicked callback is nil", func(t *testing.T) {
		config, server := newOrdersServer("boom")
		baseURL := serveOnFreePort(t, config, server.WithHooks(httpserver.OnHandlerPanicked(nil)))

		_, err := fixture.GetWithoutKeepAlive(ctx, baseURL+"/orders/42")

		require.Error(t, err, "without a hook, net/http drops the connection instead of answering 500")
	})
}

func TestOnErrorLogged(t *testing.T) {
	ctx := context.Background()

	t.Run("should receive the panic net/http logs when no OnHandlerPanicked hook exists", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		config, server := newOrdersServer("the order has no line")
		baseURL := serveOnFreePort(t, config, server.WithHooks(recorder.ErrorLoggedHook()))

		_, err := fixture.GetWithoutKeepAlive(ctx, baseURL+"/orders/42")

		require.Error(t, err)
		events := recorder.WaitForEvents(t, 1)
		require.Len(t, events, 1)
		assert.True(t, strings.HasPrefix(events[0], "error logged: http: panic serving 127.0.0.1:"),
			"unexpected event %q", events[0])
		logged := recorder.ErrorsLogged()[0].Error()
		assert.Contains(t, logged, "the order has no line")
		assert.False(t, strings.HasSuffix(logged, "\n"), "the trailing newline of the log line is trimmed")
	})

	t.Run("should not receive the panics an OnHandlerPanicked hook handled", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		config, server := newOrdersServer("boom")
		baseURL := serveOnFreePort(t, config, server.WithHooks(recorder.Hooks()...))

		response, err := fixture.GetWithoutKeepAlive(ctx, baseURL+"/orders/42")

		require.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
		assert.Equal(t, []string{"handler panicked: GET /orders/42: boom"},
			recorder.EventsContaining("handler panicked"))
		assert.Empty(t, recorder.EventsContaining("error logged"))
	})

	t.Run("should receive the TLS handshake errors net/http logs", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		config, server := newTLSServer(t)
		serveOnFreePort(t, config, server.WithHooks(recorder.ErrorLoggedHook()))

		failTLSHandshake(t, config.Address())

		events := recorder.WaitForEvents(t, 1)
		assert.True(t, strings.HasPrefix(events[0], "error logged: http: TLS handshake error from 127.0.0.1:"),
			"unexpected event %q", events[0])
	})

	t.Run("should run every hook, in registration order, across several WithHooks calls, ignoring nil",
		func(t *testing.T) {
			recorder := fixture.NewHookRecorder()
			config, server := newTLSServer(t)
			server.
				WithHooks(httpserver.OnErrorLogged(nil)).
				WithHooks(recorder.HooksLabeled("first")...).
				WithHooks(append([]httpserver.Hook{httpserver.OnErrorLogged(nil)}, recorder.HooksLabeled("second")...)...)
			serveOnFreePort(t, config, server)

			failTLSHandshake(t, config.Address())

			require.Eventually(t, func() bool { return len(recorder.EventsContaining("error logged")) == 2 },
				5*time.Second, 5*time.Millisecond, "the logged error never reached both hooks")
			events := recorder.EventsContaining("error logged")
			assert.True(t, strings.HasPrefix(events[0], "first: error logged: http: TLS handshake error from "),
				"unexpected event %q", events[0])
			assert.True(t, strings.HasPrefix(events[1], "second: error logged: http: TLS handshake error from "),
				"unexpected event %q", events[1])
		})
}

func TestOnConnectionStatusChanged(t *testing.T) {
	ctx := context.Background()

	t.Run("should report a kept-alive connection new, active, idle, then closed by Shutdown", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		config, server := newOrdersServer("boom")
		baseURL := serveOnFreePort(t, config, server.WithHooks(recorder.ConnectionStatusChangedHook()))
		var clientAddress string
		trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
			clientAddress = info.Conn.LocalAddr().String()
		}}
		request, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodGet,
			baseURL+"/orders", nil)
		require.NoError(t, err)
		client := &http.Client{Transport: &http.Transport{}}

		response, err := client.Do(request)

		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		assert.Equal(t, []string{
			"connection status changed: new",
			"connection status changed: active",
			"connection status changed: idle",
		}, recorder.WaitForEvents(t, 3))
		require.NoError(t, server.Shutdown(ctx))
		assert.Equal(t, "connection status changed: closed", recorder.WaitForEvents(t, 4)[3])
		connections := recorder.Connections()
		require.Len(t, connections, 4)
		for _, conn := range connections {
			assert.Equal(t, clientAddress, conn.RemoteAddr().String(), "the hook receives the server side")
		}
	})

	t.Run("should report a connection taken over by a handler as hijacked", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		config := &httpserver.Config{BindInterface: "127.0.0.1"}
		server := httpserver.NewServer(config, fixture.NewHijackingHandler("/chat")).
			WithHooks(recorder.ConnectionStatusChangedHook())
		serveOnFreePort(t, config, server)

		fixture.OpenHijackedConnection(t, config.Address(), "/chat")

		assert.Equal(t, []string{
			"connection status changed: new",
			"connection status changed: active",
			"connection status changed: hijacked",
		}, recorder.WaitForEvents(t, 3))
	})

	t.Run("should run every hook in registration order and ignore a nil callback", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		config, server := newOrdersServer("boom")
		server.
			WithHooks(httpserver.OnConnectionStatusChanged(nil)).
			WithHooks(recorder.HooksLabeled("first")...).
			WithHooks(append([]httpserver.Hook{httpserver.OnConnectionStatusChanged(nil)},
				recorder.HooksLabeled("second")...)...)
		baseURL := serveOnFreePort(t, config, server)

		_, err := fixture.GetWithoutKeepAlive(ctx, baseURL+"/orders")

		require.NoError(t, err)
		require.Eventually(t, func() bool { return len(recorder.EventsContaining("connection status changed")) == 6 },
			5*time.Second, 5*time.Millisecond)
		assert.Equal(t, []string{
			"first: connection status changed: new",
			"second: connection status changed: new",
			"first: connection status changed: active",
			"second: connection status changed: active",
			"first: connection status changed: closed",
			"second: connection status changed: closed",
		}, recorder.EventsContaining("connection status changed"))
	})
}

func TestOnShutdownStarted(t *testing.T) {
	ctx := context.Background()

	t.Run("should fire once when the server shuts down", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		config, server := newOrdersServer("boom")
		serveOnFreePort(t, config, server.WithHooks(recorder.ShutdownStartedHook()))
		require.Empty(t, recorder.Events(), "the hook must not fire while the server runs")

		require.NoError(t, server.Shutdown(ctx))

		// net/http runs the hook on a goroutine of its own, and Shutdown does not wait for it.
		assert.Equal(t, []string{"shutdown started"}, recorder.WaitForEvents(t, 1))
		assert.Never(t, func() bool { return len(recorder.Events()) > 1 }, quietPeriod, 5*time.Millisecond)
	})

	t.Run("should be where the hijacked connections get closed, which Shutdown leaves open", func(t *testing.T) {
		chat := fixture.NewHijackingHandler("/chat")
		config := &httpserver.Config{BindInterface: "127.0.0.1"}
		server := httpserver.NewServer(config, chat).WithHooks(httpserver.OnShutdownStarted(chat.CloseAll))
		serveOnFreePort(t, config, server)
		conn := fixture.OpenHijackedConnection(t, config.Address(), "/chat")

		require.NoError(t, server.Shutdown(ctx))

		fixture.WaitForClosedConnection(t, conn)
	})

	t.Run("should leave the hijacked connections open without a hook", func(t *testing.T) {
		chat := fixture.NewHijackingHandler("/chat")
		config := &httpserver.Config{BindInterface: "127.0.0.1"}
		server := httpserver.NewServer(config, chat)
		serveOnFreePort(t, config, server)
		conn := fixture.OpenHijackedConnection(t, config.Address(), "/chat")

		require.NoError(t, server.Shutdown(ctx))

		fixture.AssertStaysOpen(t, conn, quietPeriod)
	})

	t.Run("should run every hook in registration order and ignore a nil callback", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		config, server := newOrdersServer("boom")
		server.
			WithHooks(httpserver.OnShutdownStarted(nil)).
			WithHooks(recorder.HooksLabeled("first")...).
			WithHooks(append([]httpserver.Hook{httpserver.OnShutdownStarted(nil)}, recorder.HooksLabeled("second")...)...)
		serveOnFreePort(t, config, server)

		require.NoError(t, server.Shutdown(ctx))

		require.Eventually(t, func() bool { return len(recorder.EventsContaining("shutdown started")) == 2 },
			5*time.Second, 5*time.Millisecond)
		assert.Equal(t, []string{"first: shutdown started", "second: shutdown started"},
			recorder.EventsContaining("shutdown started"))
	})

	t.Run("should not fire when the server never started", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		_, server := newOrdersServer("boom")
		server.WithHooks(recorder.ShutdownStartedHook())

		require.NoError(t, server.Shutdown(ctx))

		assert.Never(t, func() bool { return len(recorder.Events()) > 0 }, quietPeriod, 5*time.Millisecond)
	})
}

// newTLSServer returns the configuration of a loopback server serving TLS with a self-signed certificate, and a
// server without route built from it.
func newTLSServer(t *testing.T) (*httpserver.Config, *httpserver.Server) {
	t.Helper()
	certFile, keyFile := fixture.WriteSelfSignedCertificate(t)
	config := &httpserver.Config{BindInterface: "127.0.0.1", TLSCertFile: certFile, TLSKeyFile: keyFile}
	return config, httpserver.NewServer(config)
}

// failTLSHandshake sends address bytes that are neither TLS nor HTTP, which fails the handshake net/http logs,
// and waits until the server drops the connection.
func failTLSHandshake(t *testing.T, address string) {
	t.Helper()
	var dialer net.Dialer
	conn, err := dialer.DialContext(context.Background(), "tcp", address)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = conn.Write([]byte("hello, TLS?\r\n"))
	require.NoError(t, err)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, _ = io.Copy(io.Discard, conn)
}
