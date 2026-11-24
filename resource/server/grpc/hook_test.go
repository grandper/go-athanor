package grpc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"

	athanorfixture "github.com/grandper/go-athanor/fixture"
	"github.com/grandper/go-athanor/resource"
	grpcserver "github.com/grandper/go-athanor/resource/server/grpc"
	"github.com/grandper/go-athanor/resource/server/grpc/fixture"
)

// healthCheckMethod is the full method name of the Check of the gRPC health service.
const healthCheckMethod = "/grpc.health.v1.Health/Check"

// serveOnFreePort starts server on a free port of the loopback interface, shuts it down when the test ends, and
// returns a health client of it.
func serveOnFreePort(t *testing.T, server *grpcserver.Server) grpc_health_v1.HealthClient {
	t.Helper()
	ctx := context.Background()
	listener := athanorfixture.ListenOnFreePort(t)
	require.NoError(t, server.WithListener(listener).Init(ctx))
	t.Cleanup(func() { _ = server.Shutdown(ctx) })
	return fixture.NewHealthClient(t, listener.Addr().String())
}

// newHealthServer returns a server of the health service of an empty resource list, for which every named service
// is unknown.
func newHealthServer() *grpcserver.Server {
	return grpcserver.NewServer(&grpcserver.Config{}, grpcserver.NewHealthHandler(resource.NewList()))
}

func TestOnHandlerFailed(t *testing.T) {
	ctx := context.Background()

	t.Run("should receive the method and the error of a unary request that fails", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		client := serveOnFreePort(t, newHealthServer().WithHooks(recorder.HandlerFailedHook()))

		_, err := checkHealth(t, client, "orders")

		require.Equal(t, codes.NotFound, status.Code(err))
		// The hook runs before the response is sent, so it already ran.
		assert.Equal(t, []string{"handler failed: /grpc.health.v1.Health/Check: NotFound"}, recorder.Events())
		failure := recorder.Failures()[0]
		assert.Equal(t, healthCheckMethod, failure.Method)
		assert.Equal(t, codes.NotFound, status.Code(failure.Err))
		assert.Equal(t, "unknown service", status.Convert(failure.Err).Message())
		assert.NotNil(t, failure.Ctx)
	})

	t.Run("should receive the method and the error of a stream request that fails", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		client := serveOnFreePort(t, newHealthServer().WithHooks(recorder.HandlerFailedHook()))
		watchCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		stream, err := client.Watch(watchCtx, &grpc_health_v1.HealthCheckRequest{})
		require.NoError(t, err)
		_, err = stream.Recv()
		require.NoError(t, err)

		cancel() // the watcher leaves: the Watch handler ends with codes.Canceled

		assert.Equal(t, []string{"handler failed: /grpc.health.v1.Health/Watch: Canceled"},
			recorder.WaitForEvents(t, 1))
		assert.Equal(t, "/grpc.health.v1.Health/Watch", recorder.Failures()[0].Method)
	})

	t.Run("should not fire for a request that succeeds", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		client := serveOnFreePort(t, newHealthServer().WithHooks(recorder.Hooks()...))

		response, err := checkHealth(t, client, "")

		require.NoError(t, err)
		assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, response.GetStatus())
		assert.Empty(t, recorder.EventsContaining("handler "))
	})

	t.Run("should run every hook, in registration order, across several WithHooks calls", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		server := newHealthServer().
			WithHooks(recorder.HooksLabeled("first")...).
			WithHooks(append(recorder.HooksLabeled("second"), recorder.HooksLabeled("third")...)...)
		client := serveOnFreePort(t, server)

		_, err := checkHealth(t, client, "orders")

		require.Error(t, err)
		assert.Equal(t, []string{
			"first: handler failed: /grpc.health.v1.Health/Check: NotFound",
			"second: handler failed: /grpc.health.v1.Health/Check: NotFound",
			"third: handler failed: /grpc.health.v1.Health/Check: NotFound",
		}, recorder.EventsContaining("handler "))
	})

	t.Run("should ignore a nil callback and still run the other hooks", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		server := newHealthServer().
			WithHooks(grpcserver.OnHandlerFailed(nil), grpcserver.OnHandlerPanicked(nil)).
			WithHooks(recorder.Hooks()...).
			WithHooks(grpcserver.OnHandlerFailed(nil))
		client := serveOnFreePort(t, server)

		_, err := checkHealth(t, client, "orders")

		require.Equal(t, codes.NotFound, status.Code(err), "a nil callback must neither panic nor change the error")
		assert.Equal(t, []string{"handler failed: /grpc.health.v1.Health/Check: NotFound"},
			recorder.EventsContaining("handler "))
	})
}

func TestOnHandlerPanicked(t *testing.T) {
	t.Run("should fail the request with codes.Internal and hand the hook the method and the recovered value",
		func(t *testing.T) {
			recorder := fixture.NewHookRecorder()
			server := grpcserver.NewServer(&grpcserver.Config{}, fixture.NewPanickingHandler("the order has no line")).
				WithHooks(recorder.HandlerPanickedHook())
			client := serveOnFreePort(t, server)

			_, err := checkHealth(t, client, "")

			require.Equal(t, codes.Internal, status.Code(err))
			assert.Equal(t, "internal error", status.Convert(err).Message())
			assert.Equal(t, []string{"handler panicked: /grpc.health.v1.Health/Check: the order has no line"},
				recorder.Events())
			panics := recorder.Panics()
			require.Len(t, panics, 1)
			assert.Equal(t, healthCheckMethod, panics[0].Method)
			assert.Equal(t, "the order has no line", panics[0].Recovered)
			assert.NotNil(t, panics[0].Ctx)
		})

	t.Run("should hand the hook the error a handler panicked with", func(t *testing.T) {
		errPanic := errors.New("nil order")
		recorder := fixture.NewHookRecorder()
		server := grpcserver.NewServer(&grpcserver.Config{}, fixture.NewPanickingHandler(errPanic)).
			WithHooks(recorder.HandlerPanickedHook())
		client := serveOnFreePort(t, server)

		_, err := checkHealth(t, client, "")

		require.Equal(t, codes.Internal, status.Code(err))
		recovered, ok := recorder.Panics()[0].Recovered.(error)
		require.True(t, ok, "the recovered value must be the error itself")
		assert.ErrorIs(t, recovered, errPanic)
	})

	t.Run("should let OnHandlerFailed see the codes.Internal error of the recovered panic", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		server := grpcserver.NewServer(&grpcserver.Config{}, fixture.NewPanickingHandler("boom")).
			WithHooks(recorder.Hooks()...)
		client := serveOnFreePort(t, server)

		_, err := checkHealth(t, client, "")

		require.Equal(t, codes.Internal, status.Code(err))
		// The failure hooks wrap the panic recovery, so they see the panic after it was turned into an error.
		assert.Equal(t, []string{
			"handler panicked: /grpc.health.v1.Health/Check: boom",
			"handler failed: /grpc.health.v1.Health/Check: Internal",
		}, recorder.EventsContaining("handler "))
	})

	t.Run("should run every panic hook, in registration order, and ignore a nil callback", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		server := grpcserver.NewServer(&grpcserver.Config{}, fixture.NewPanickingHandler("boom")).
			WithHooks(grpcserver.OnHandlerPanicked(nil)).
			WithHooks(recorder.HooksLabeled("first")...).
			WithHooks(recorder.HooksLabeled("second")...)
		client := serveOnFreePort(t, server)

		_, err := checkHealth(t, client, "")

		require.Equal(t, codes.Internal, status.Code(err), "a nil callback must neither panic nor stop the recovery")
		assert.Equal(t, []string{
			"first: handler panicked: /grpc.health.v1.Health/Check: boom",
			"second: handler panicked: /grpc.health.v1.Health/Check: boom",
			"first: handler failed: /grpc.health.v1.Health/Check: Internal",
			"second: handler failed: /grpc.health.v1.Health/Check: Internal",
		}, recorder.EventsContaining("handler "))
	})

	t.Run("should keep serving after a panic", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		server := grpcserver.NewServer(&grpcserver.Config{}, fixture.NewPanickingHandler("boom")).
			WithHooks(recorder.HandlerPanickedHook())
		client := serveOnFreePort(t, server)
		_, err := checkHealth(t, client, "")
		require.Equal(t, codes.Internal, status.Code(err))

		_, err = checkHealth(t, client, "")

		require.Equal(t, codes.Internal, status.Code(err), "the server must still answer")
		assert.Len(t, recorder.Panics(), 2)
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
	})

	t.Run("should recover a panicking stream handler too", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		server := grpcserver.NewServer(&grpcserver.Config{}, fixture.NewPanickingHandler("boom")).
			WithHooks(recorder.Hooks()...)
		client := serveOnFreePort(t, server)
		watchCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stream, err := client.Watch(watchCtx, &grpc_health_v1.HealthCheckRequest{})
		require.NoError(t, err)

		_, err = stream.Recv()

		require.Equal(t, codes.Internal, status.Code(err))
		assert.Equal(t, []string{
			"handler panicked: /grpc.health.v1.Health/Watch: boom",
			"handler failed: /grpc.health.v1.Health/Watch: Internal",
		}, recorder.EventsContaining("handler "))
	})
}

// serveForConnection starts server on a free port of the loopback interface, shuts it down when the test ends,
// and returns a connection to it that remembers its local address.
func serveForConnection(t *testing.T, server *grpcserver.Server) *fixture.HealthConnection {
	t.Helper()
	ctx := context.Background()
	listener := athanorfixture.ListenOnFreePort(t)
	require.NoError(t, server.WithListener(listener).Init(ctx))
	t.Cleanup(func() { _ = server.Shutdown(ctx) })
	return fixture.NewHealthConnection(t, listener.Addr().String())
}

func TestOnConnectionEstablished(t *testing.T) {
	t.Run("should fire with the remote address of the client once its connection is established", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		connection := serveForConnection(t, newHealthServer().WithHooks(recorder.ConnectionEstablishedHook()))

		_, err := checkHealth(t, connection.Client(), "")

		require.NoError(t, err)
		assert.Equal(t, []string{"connection established"}, recorder.WaitForEvents(t, 1))
		assert.Equal(t, connection.LocalAddress(), recorder.ConnectionInfos()[0].RemoteAddr.String())
	})

	t.Run("should not fire before a client connects", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		serveForConnection(t, newHealthServer().WithHooks(recorder.ConnectionEstablishedHook()))

		assert.Never(t, func() bool { return len(recorder.Events()) > 0 }, 100*time.Millisecond, 5*time.Millisecond)
	})
}

func TestOnConnectionClosed(t *testing.T) {
	ctx := context.Background()

	t.Run("should fire with the remote address of the client when the client closes its connection",
		func(t *testing.T) {
			recorder := fixture.NewHookRecorder()
			connection := serveForConnection(t, newHealthServer().
				WithHooks(recorder.ConnectionEstablishedHook(), recorder.ConnectionClosedHook()))
			_, err := checkHealth(t, connection.Client(), "")
			require.NoError(t, err)

			require.NoError(t, connection.Close())

			assert.Equal(t, []string{"connection established", "connection closed"}, recorder.WaitForEvents(t, 2))
			infos := recorder.ConnectionInfos()
			assert.Equal(t, connection.LocalAddress(), infos[1].RemoteAddr.String())
		})

	t.Run("should fire for the connections Shutdown closes", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		server := newHealthServer().WithHooks(recorder.ConnectionClosedHook())
		connection := serveForConnection(t, server)
		_, err := checkHealth(t, connection.Client(), "")
		require.NoError(t, err)

		require.NoError(t, server.Shutdown(ctx))

		// gRPC may report the closed connection after GracefulStop returns, and Shutdown does not wait for it.
		assert.Equal(t, []string{"connection closed"}, recorder.WaitForEvents(t, 1))
		assert.Equal(t, connection.LocalAddress(), recorder.ConnectionInfos()[0].RemoteAddr.String())
	})

	t.Run("should run every connection hook in registration order and ignore a nil callback", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		server := newHealthServer().
			WithHooks(grpcserver.OnConnectionEstablished(nil), grpcserver.OnConnectionClosed(nil)).
			WithHooks(recorder.HooksLabeled("first")...).
			WithHooks(append([]grpcserver.Hook{grpcserver.OnConnectionClosed(nil)},
				recorder.HooksLabeled("second")...)...)
		connection := serveForConnection(t, server)
		_, err := checkHealth(t, connection.Client(), "")
		require.NoError(t, err)

		require.NoError(t, connection.Close())

		assert.Equal(t, []string{
			"first: connection established",
			"second: connection established",
			"first: connection closed",
			"second: connection closed",
		}, recorder.WaitForEventsContaining(t, "connection", 4))
	})
}

func TestOnRequestStatsReported(t *testing.T) {
	t.Run("should report a request beginning first and ending last", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		client := serveOnFreePort(t, newHealthServer().WithHooks(recorder.RequestStatsReportedHook()))

		_, err := checkHealth(t, client, "")

		require.NoError(t, err)
		// The end of the request is reported once the response is sent, so it may come after the client got it.
		recorder.WaitForEventsContaining(t, "*stats.End", 1)
		requestStats := recorder.RequestStats()
		require.NotEmpty(t, requestStats)
		_, ended := requestStats[len(requestStats)-1].(*stats.End)
		assert.True(t, ended, "the last event must be a *stats.End, got %T", requestStats[len(requestStats)-1])
		begins := recorder.EventsContaining("*stats.Begin")
		assert.Equal(t, []string{"request stats reported: *stats.Begin"}, begins)
		events := recorder.Events()
		assert.Less(t, indexOf(events, "request stats reported: *stats.Begin"),
			indexOf(events, "request stats reported: *stats.End"), "the request begins before it ends")
		for _, requestStat := range requestStats {
			assert.False(t, requestStat.IsClient(), "the server reports its own side of the request")
		}
	})

	t.Run("should report the end of a failed request with its error", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		client := serveOnFreePort(t, newHealthServer().WithHooks(recorder.RequestStatsReportedHook()))

		_, err := checkHealth(t, client, "orders")

		require.Equal(t, codes.NotFound, status.Code(err))
		recorder.WaitForEventsContaining(t, "*stats.End", 1)
		requestStats := recorder.RequestStats()
		end, ok := requestStats[len(requestStats)-1].(*stats.End)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, status.Code(end.Error))
	})

	t.Run("should run every hook in registration order and ignore a nil callback", func(t *testing.T) {
		recorder := fixture.NewHookRecorder()
		server := newHealthServer().
			WithHooks(grpcserver.OnRequestStatsReported(nil)).
			WithHooks(recorder.HooksLabeled("first")...).
			WithHooks(append([]grpcserver.Hook{grpcserver.OnRequestStatsReported(nil)},
				recorder.HooksLabeled("second")...)...)
		client := serveOnFreePort(t, server)

		_, err := checkHealth(t, client, "")

		require.NoError(t, err)
		assert.Equal(t, []string{
			"first: request stats reported: *stats.End",
			"second: request stats reported: *stats.End",
		}, recorder.WaitForEventsContaining(t, "*stats.End", 2))
		assert.Equal(t, []string{
			"first: request stats reported: *stats.Begin",
			"second: request stats reported: *stats.Begin",
		}, recorder.EventsContaining("*stats.Begin"))
	})
}

// indexOf returns the index of the first event equal to event, or -1 when there is none.
func indexOf(events []string, event string) int {
	for i, candidate := range events {
		if candidate == event {
			return i
		}
	}
	return -1
}
