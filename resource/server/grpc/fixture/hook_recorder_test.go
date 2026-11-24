package fixture_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	athanorfixture "github.com/grandper/go-athanor/fixture"
	"github.com/grandper/go-athanor/resource"
	grpcserver "github.com/grandper/go-athanor/resource/server/grpc"
	"github.com/grandper/go-athanor/resource/server/grpc/fixture"
)

// serveWithHooks starts a server of handler on a free port, hooked with hooks, and returns a health client of
// it. The server is shut down when the test ends.
func serveWithHooks(t *testing.T, handler grpcserver.Handler, hooks ...grpcserver.Hook) grpc_health_v1.HealthClient {
	t.Helper()
	ctx := context.Background()
	listener := athanorfixture.ListenOnFreePort(t)
	server := grpcserver.NewServer(&grpcserver.Config{}, handler).WithListener(listener).WithHooks(hooks...)
	require.NoError(t, server.Init(ctx))
	t.Cleanup(func() { _ = server.Shutdown(ctx) })
	return fixture.NewHealthClient(t, listener.Addr().String())
}

// check asks client for the health of service, within a bounded time.
func check(client grpc_health_v1.HealthClient, service string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: service})
	return err
}

func TestHookRecorder(t *testing.T) {
	t.Run("should record nothing before the server reports an event", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()

		assert.Empty(t, recorder.Events())
		assert.Empty(t, recorder.Failures())
		assert.Empty(t, recorder.Panics())
		assert.Empty(t, recorder.ConnectionInfos())
		assert.Empty(t, recorder.RequestStats())
	})

	t.Run("should return one hook per lifecycle event", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()

		assert.Len(t, recorder.Hooks(), 5)
		assert.Len(t, recorder.HooksLabeled("first"), 5)
	})

	t.Run("should record a failure with its method and gRPC code", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		resources, err := resource.NewList()
		require.NoError(t, err)
		client := serveWithHooks(
			t,
			grpcserver.NewHealthHandler(resources),
			recorder.HandlerFailedHook(),
		)

		err = check(client, "orders")

		require.Equal(t, codes.NotFound, status.Code(err))
		assert.Equal(t, []string{"handler failed: /grpc.health.v1.Health/Check: NotFound"}, recorder.Events())
		failures := recorder.Failures()
		require.Len(t, failures, 1)
		assert.Equal(t, "/grpc.health.v1.Health/Check", failures[0].Method)
		assert.Equal(t, codes.NotFound, status.Code(failures[0].Err))
		assert.NotNil(t, failures[0].Ctx)
	})

	t.Run("should record a panic with its method and recovered value", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		client := serveWithHooks(t, fixture.NewPanickingHandler("the order has no line"),
			recorder.HandlerPanickedHook())

		_ = check(client, "")

		assert.Equal(t, []string{"handler panicked: /grpc.health.v1.Health/Check: the order has no line"},
			recorder.Events())
		panics := recorder.Panics()
		require.Len(t, panics, 1)
		assert.Equal(t, "the order has no line", panics[0].Recovered)
		assert.NotNil(t, panics[0].Ctx)
	})

	t.Run("should prefix the events of labeled hooks with their label", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		resources, err := resource.NewList()
		require.NoError(t, err)
		client := serveWithHooks(t, grpcserver.NewHealthHandler(resources),
			append(recorder.HooksLabeled("first"), recorder.HooksLabeled("second")...)...)

		_ = check(client, "orders")

		assert.Equal(t, []string{
			"first: handler failed: /grpc.health.v1.Health/Check: NotFound",
			"second: handler failed: /grpc.health.v1.Health/Check: NotFound",
		}, recorder.EventsContaining("handler "))
	})

	t.Run("should return the events containing a text, in order", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		client := serveWithHooks(t, fixture.NewPanickingHandler("boom"), recorder.Hooks()...)

		_ = check(client, "")

		assert.Equal(t, []string{"handler panicked: /grpc.health.v1.Health/Check: boom"},
			recorder.EventsContaining("panicked"))
		assert.Equal(t, []string{"handler failed: /grpc.health.v1.Health/Check: Internal"},
			recorder.EventsContaining("failed"))
	})

	t.Run("should wait for the events reported after the response", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		resources, err := resource.NewList()
		require.NoError(t, err)
		client := serveWithHooks(
			t,
			grpcserver.NewHealthHandler(resources),
			recorder.HandlerFailedHook(),
		)
		ctx, cancel := context.WithCancel(context.Background())
		stream, err := client.Watch(ctx, &grpc_health_v1.HealthCheckRequest{})
		require.NoError(t, err)
		_, err = stream.Recv()
		require.NoError(t, err)

		cancel()

		assert.Equal(t, []string{"handler failed: /grpc.health.v1.Health/Watch: Canceled"},
			recorder.WaitForEvents(t, 1))
	})

	t.Run("should return a snapshot of the events", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		resources, err := resource.NewList()
		require.NoError(t, err)
		client := serveWithHooks(
			t,
			grpcserver.NewHealthHandler(resources),
			recorder.HandlerFailedHook(),
		)
		_ = check(client, "orders")
		events := recorder.Events()

		events[0] = "changed"

		assert.Equal(t, []string{"handler failed: /grpc.health.v1.Health/Check: NotFound"}, recorder.Events())
	})

	t.Run("should record the connection established, then closed, with the information of the connection",
		func(t *testing.T) {
			recorder := fixture.NewHookRecorder()
			listener := athanorfixture.ListenOnFreePort(t)
			resources, err := resource.NewList()
			require.NoError(t, err)
			server := grpcserver.NewServer(&grpcserver.Config{}, grpcserver.NewHealthHandler(resources)).
				WithListener(listener).
				WithHooks(recorder.ConnectionEstablishedHook(), recorder.ConnectionClosedHook())
			require.NoError(t, server.Init(context.Background()))
			t.Cleanup(func() { _ = server.Shutdown(context.Background()) })
			connection := fixture.NewHealthConnection(t, listener.Addr().String())
			require.NoError(t, check(connection.Client(), ""))

			require.NoError(t, connection.Close())

			assert.Equal(t, []string{"connection established", "connection closed"}, recorder.WaitForEvents(t, 2))
			infos := recorder.ConnectionInfos()
			require.Len(t, infos, 2)
			assert.Equal(t, connection.LocalAddress(), infos[0].RemoteAddr.String())
			assert.Same(t, infos[0], infos[1], "the closed connection is reported with the information it was tagged")
		})

	t.Run("should record the statistics events of a request with their type", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		resources, err := resource.NewList()
		require.NoError(t, err)
		client := serveWithHooks(t, grpcserver.NewHealthHandler(resources),
			recorder.RequestStatsReportedHook())

		require.NoError(t, check(client, ""))

		assert.Equal(t, []string{"request stats reported: *stats.End"},
			recorder.WaitForEventsContaining(t, "*stats.End", 1))
		assert.Equal(t, []string{"request stats reported: *stats.Begin"}, recorder.EventsContaining("*stats.Begin"))
		assert.Len(t, recorder.RequestStats(), len(recorder.Events()))
	})

	t.Run("should wait for the events containing a text", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		resources, err := resource.NewList()
		require.NoError(t, err)
		client := serveWithHooks(t, grpcserver.NewHealthHandler(resources), recorder.Hooks()...)

		_ = check(client, "orders")

		assert.Equal(t, []string{"handler failed: /grpc.health.v1.Health/Check: NotFound"},
			recorder.WaitForEventsContaining(t, "handler failed", 1))
	})
}
