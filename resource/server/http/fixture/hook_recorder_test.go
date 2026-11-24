package fixture_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpserver "github.com/grandper/go-athanor/resource/server/http"
	"github.com/grandper/go-athanor/resource/server/http/fixture"
)

// servePanickingRoute starts a server whose route /orders/42 panics with value, hooked with hooks, and returns
// the URL of the route. The server is shut down when the test ends.
func servePanickingRoute(t *testing.T, value any, hooks ...httpserver.Hook) string {
	t.Helper()
	ctx := context.Background()
	config := &httpserver.Config{BindInterface: "127.0.0.1"}
	server := httpserver.NewServer(config, fixture.NewPanickingHandler("/orders/42", value)).WithHooks(hooks...)
	require.NoError(t, fixture.InitOnFreePort(ctx, t, config, server.Init))
	t.Cleanup(func() { _ = server.Shutdown(ctx) })
	return fmt.Sprintf("http://%s/orders/42", config.Address())
}

func TestHookRecorder(t *testing.T) {
	ctx := context.Background()

	t.Run("should record nothing before the server reports an event", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()

		assert.Empty(t, recorder.Events())
		assert.Empty(t, recorder.Panics())
		assert.Empty(t, recorder.ErrorsLogged())
		assert.Empty(t, recorder.Connections())
	})

	t.Run("should return one hook per lifecycle event", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()

		assert.Len(t, recorder.Hooks(), 4)
		assert.Len(t, recorder.HooksLabeled("first"), 4)
	})

	t.Run("should record a panic with its request and recovered value", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		url := servePanickingRoute(t, "the order has no line", recorder.HandlerPanickedHook())

		_, _ = fixture.GetWithoutKeepAlive(ctx, url)

		assert.Equal(t, []string{"handler panicked: GET /orders/42: the order has no line"},
			recorder.WaitForEvents(t, 1))
		panics := recorder.Panics()
		require.Len(t, panics, 1)
		assert.Equal(t, "/orders/42", panics[0].Request.URL.Path)
		assert.Equal(t, "the order has no line", panics[0].Recovered)
	})

	t.Run("should record the first line of a logged error, and keep the whole error", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		url := servePanickingRoute(t, "the order has no line", recorder.ErrorLoggedHook())

		_, _ = fixture.GetWithoutKeepAlive(ctx, url)

		events := recorder.WaitForEvents(t, 1)
		require.Len(t, events, 1)
		assert.True(t, strings.HasPrefix(events[0], "error logged: http: panic serving "), "unexpected %q", events[0])
		assert.True(t, strings.HasSuffix(events[0], ": the order has no line"), "unexpected %q", events[0])
		assert.NotContains(t, events[0], "\n")
		assert.Contains(t, recorder.ErrorsLogged()[0].Error(), "goroutine", "the whole error keeps the stack trace")
	})

	t.Run("should prefix the events of labeled hooks with their label", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		url := servePanickingRoute(t, "boom",
			append(recorder.HooksLabeled("first"), recorder.HooksLabeled("second")...)...)

		_, _ = fixture.GetWithoutKeepAlive(ctx, url)

		recorder.WaitForEvents(t, 2)
		// The connection status changes are reported too: only the panics are compared.
		assert.Equal(t, []string{
			"first: handler panicked: GET /orders/42: boom",
			"second: handler panicked: GET /orders/42: boom",
		}, recorder.EventsContaining("handler panicked"))
	})

	t.Run("should return a snapshot of the events", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		url := servePanickingRoute(t, "boom", recorder.HandlerPanickedHook())
		_, _ = fixture.GetWithoutKeepAlive(ctx, url)
		events := recorder.WaitForEvents(t, 1)

		events[0] = "changed"

		assert.Equal(t, []string{"handler panicked: GET /orders/42: boom"}, recorder.Events())
	})

	t.Run("should record the status changes of a connection, with the connection", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		url := servePanickingRoute(t, "boom", recorder.ConnectionStatusChangedHook())

		_, _ = fixture.GetWithoutKeepAlive(ctx, strings.TrimSuffix(url, "/orders/42")+"/missing")

		// A connection the client does not keep alive goes from new to active, then closed.
		assert.Equal(t, []string{
			"connection status changed: new",
			"connection status changed: active",
			"connection status changed: closed",
		}, recorder.WaitForEvents(t, 3))
		connections := recorder.Connections()
		require.Len(t, connections, 3)
		assert.Same(t, connections[0], connections[2], "every change is reported with the same connection")
	})

	t.Run("should record the start of the shutdown", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		config := &httpserver.Config{BindInterface: "127.0.0.1"}
		server := httpserver.NewServer(config).WithHooks(recorder.ShutdownStartedHook())
		require.NoError(t, fixture.InitOnFreePort(ctx, t, config, server.Init))

		require.NoError(t, server.Shutdown(ctx))

		assert.Equal(t, []string{"shutdown started"}, recorder.WaitForEvents(t, 1))
	})

	t.Run("should return the events containing a text, in order", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		url := servePanickingRoute(t, "boom", recorder.Hooks()...)
		_, _ = fixture.GetWithoutKeepAlive(ctx, url)
		_, _ = fixture.GetWithoutKeepAlive(ctx, url)
		require.Eventually(t, func() bool { return len(recorder.EventsContaining("handler panicked")) == 2 },
			time.Second, 5*time.Millisecond)

		assert.Equal(t, []string{
			"handler panicked: GET /orders/42: boom",
			"handler panicked: GET /orders/42: boom",
		}, recorder.EventsContaining("handler panicked"))
		assert.Empty(t, recorder.EventsContaining("shutdown"))
	})
}
