package fixture_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	athanorfixture "github.com/grandper/go-athanor/fixture"
	"github.com/grandper/go-athanor/resource"
	grpcserver "github.com/grandper/go-athanor/resource/server/grpc"
	"github.com/grandper/go-athanor/resource/server/grpc/fixture"
)

func TestNewHealthClient(t *testing.T) {
	ctx := context.Background()

	t.Run("should return a client of the health service served on the address", func(t *testing.T) {
		listener := athanorfixture.ListenOnFreePort(t)
		server := grpcserver.NewServer(&grpcserver.Config{}, grpcserver.NewHealthHandler(resource.NewList())).
			WithListener(listener)
		require.NoError(t, server.Init(ctx))
		defer func() { require.NoError(t, server.Shutdown(ctx)) }()

		client := fixture.NewHealthClient(t, listener.Addr().String())

		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		response, err := client.Check(checkCtx, &grpc_health_v1.HealthCheckRequest{})
		require.NoError(t, err)
		assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, response.GetStatus())
	})
}

func TestAssertEventuallyServing(t *testing.T) {
	ctx := context.Background()

	t.Run("should accept a service reported as serving", func(t *testing.T) {
		listener := athanorfixture.ListenOnFreePort(t)
		server := grpcserver.NewServer(&grpcserver.Config{}, grpcserver.NewHealthHandler(resource.NewList())).
			WithListener(listener)
		require.NoError(t, server.Init(ctx))
		defer func() { require.NoError(t, server.Shutdown(ctx)) }()

		fixture.AssertEventuallyServing(t, fixture.NewHealthClient(t, listener.Addr().String()), "")
	})
}

// serveStandardHealth serves the standard gRPC health service on a free port, stopped when the test ends, and
// returns its address.
func serveStandardHealth(t *testing.T) string {
	t.Helper()
	listener := athanorfixture.ListenOnFreePort(t)
	server := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(server, health.NewServer())
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return listener.Addr().String()
}

func TestHealthConnection(t *testing.T) {
	t.Run("should reach the health service and remember the local address it dialed from", func(t *testing.T) {
		address := serveStandardHealth(t)
		connection := fixture.NewHealthConnection(t, address)
		require.Empty(t, connection.LocalAddress(), "a gRPC client dials on its first request")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		response, err := connection.Client().Check(ctx, &grpc_health_v1.HealthCheckRequest{})

		require.NoError(t, err)
		assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, response.GetStatus())
		assert.Contains(t, connection.LocalAddress(), "127.0.0.1:")
		assert.NotEqual(t, address, connection.LocalAddress())
	})

	t.Run("should close the connection", func(t *testing.T) {
		connection := fixture.NewHealthConnection(t, serveStandardHealth(t))

		require.NoError(t, connection.Close())

		assert.Error(t, connection.Close(), "the connection is already closed")
	})
}

func TestFakeHealthWatchStream(t *testing.T) {
	ctx := context.Background()

	t.Run("should record the sent responses", func(t *testing.T) {
		stream := fixture.NewFakeHealthWatchStream(ctx)
		response := &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}

		err := stream.Send(response)

		require.NoError(t, err)
		assert.Equal(t, []*grpc_health_v1.HealthCheckResponse{response}, stream.Sent())
	})

	t.Run("should expose the context given at construction", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		stream := fixture.NewFakeHealthWatchStream(cancelCtx)

		cancel()

		assert.ErrorIs(t, stream.Context().Err(), context.Canceled)
	})

	t.Run("fails to send with the configured error", func(t *testing.T) {
		errSend := errors.New("send failed")
		stream := fixture.NewFakeHealthWatchStream(ctx).FailSendWith(errSend)

		err := stream.Send(&grpc_health_v1.HealthCheckResponse{})

		require.ErrorIs(t, err, errSend)
		assert.Empty(t, stream.Sent())
	})
}
