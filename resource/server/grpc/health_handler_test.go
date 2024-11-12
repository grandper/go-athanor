package grpc_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/grandper/go-athanor/resource"
	"github.com/grandper/go-athanor/resource/fixture"
	grpcserver "github.com/grandper/go-athanor/resource/server/grpc"
	grpcfixture "github.com/grandper/go-athanor/resource/server/grpc/fixture"
)

// startHealthServer serves a health handler in memory and returns a client wired to it.
func startHealthServer(t *testing.T, handler *grpcserver.HealthHandler) grpc_health_v1.HealthClient {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	require.NoError(t, handler.Register(server))

	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	dialer := func(context.Context, string) (net.Conn, error) { return listener.Dial() }
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return grpc_health_v1.NewHealthClient(conn)
}

// checkHealth asks the health service for the status of service, within a bounded time.
func checkHealth(t *testing.T, client grpc_health_v1.HealthClient, service string,
) (*grpc_health_v1.HealthCheckResponse, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return client.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: service})
}

// watchHealth opens a Watch stream on service, closed when the test ends.
func watchHealth(t *testing.T, client grpc_health_v1.HealthClient, service string) grpc_health_v1.Health_WatchClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	stream, err := client.Watch(ctx, &grpc_health_v1.HealthCheckRequest{Service: service})
	require.NoError(t, err)
	return stream
}

// requireNextStatus asserts that the next status streamed by a Watch is expected.
func requireNextStatus(t *testing.T, stream grpc_health_v1.Health_WatchClient,
	expected grpc_health_v1.HealthCheckResponse_ServingStatus,
) {
	t.Helper()
	response, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, expected, response.GetStatus())
}

func TestHealthHandler(t *testing.T) {
	ctx := context.Background()

	t.Run("should report the server as serving when all resources are operational", func(t *testing.T) {
		resources := resource.NewList()
		require.NoError(t, resources.Register(fixture.NewHealthyFakeManaged("db")))
		require.NoError(t, resources.Register(fixture.NewHealthyFakeManaged("nats")))
		require.NoError(t, resources.Init(ctx))
		client := startHealthServer(t, grpcserver.NewHealthHandler(resources))

		response, err := checkHealth(t, client, "")

		require.NoError(t, err)
		assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, response.GetStatus())
	})

	t.Run("should report the server as not serving when a resource is not operational", func(t *testing.T) {
		resources := resource.NewList()
		db := fixture.NewHealthyFakeManaged("db")
		require.NoError(t, resources.Register(db))
		require.NoError(t, resources.Register(fixture.NewHealthyFakeManaged("nats")))
		require.NoError(t, resources.Init(ctx))
		db.Notify(resource.StatusUnhealthy)
		client := startHealthServer(t, grpcserver.NewHealthHandler(resources))

		response, err := checkHealth(t, client, "")

		require.NoError(t, err)
		assert.Equal(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING, response.GetStatus())
	})

	t.Run("should report the server as serving for a list with no resources", func(t *testing.T) {
		client := startHealthServer(t, grpcserver.NewHealthHandler(resource.NewList()))

		response, err := checkHealth(t, client, "")

		require.NoError(t, err)
		assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, response.GetStatus())
	})

	t.Run("should report every resource as a service named after it", func(t *testing.T) {
		resources := resource.NewList()
		db := fixture.NewHealthyFakeManaged("db")
		require.NoError(t, resources.Register(db))
		require.NoError(t, resources.Register(fixture.NewHealthyFakeManaged("nats")))
		require.NoError(t, resources.Init(ctx))
		db.Notify(resource.StatusUnhealthy)
		client := startHealthServer(t, grpcserver.NewHealthHandler(resources))

		dbResponse, err := checkHealth(t, client, "db")
		require.NoError(t, err)
		natsResponse, err := checkHealth(t, client, "nats")
		require.NoError(t, err)

		assert.Equal(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING, dbResponse.GetStatus())
		assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, natsResponse.GetStatus())
	})

	t.Run("fails the check for an unknown service", func(t *testing.T) {
		client := startHealthServer(t, grpcserver.NewHealthHandler(resource.NewList()))

		_, err := checkHealth(t, client, "unknown-service")

		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("should stream the health of the server to a watcher whenever it changes", func(t *testing.T) {
		resources := resource.NewList()
		db := fixture.NewHealthyFakeManaged("db")
		require.NoError(t, resources.Register(db))
		require.NoError(t, resources.Init(ctx))
		client := startHealthServer(t, grpcserver.NewHealthHandler(resources))

		stream := watchHealth(t, client, "")

		requireNextStatus(t, stream, grpc_health_v1.HealthCheckResponse_SERVING)
		db.Notify(resource.StatusUnhealthy)
		requireNextStatus(t, stream, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
		db.Notify(resource.StatusHealthy)
		requireNextStatus(t, stream, grpc_health_v1.HealthCheckResponse_SERVING)
	})

	t.Run("should only stream the changes of the serving status", func(t *testing.T) {
		resources := resource.NewList()
		db := fixture.NewHealthyFakeManaged("db")
		require.NoError(t, resources.Register(db))
		require.NoError(t, resources.Init(ctx))
		client := startHealthServer(t, grpcserver.NewHealthHandler(resources))

		stream := watchHealth(t, client, "")

		requireNextStatus(t, stream, grpc_health_v1.HealthCheckResponse_SERVING)
		db.Notify(resource.StatusDegraded) // still operational: nothing to stream
		db.Notify(resource.StatusFailed)
		requireNextStatus(t, stream, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	})

	t.Run("should stream the health of a single resource to a watcher", func(t *testing.T) {
		resources := resource.NewList()
		db := fixture.NewHealthyFakeManaged("db")
		require.NoError(t, resources.Register(db))
		require.NoError(t, resources.Init(ctx))
		client := startHealthServer(t, grpcserver.NewHealthHandler(resources))

		stream := watchHealth(t, client, "db")

		requireNextStatus(t, stream, grpc_health_v1.HealthCheckResponse_SERVING)
		db.Notify(resource.StatusUnhealthy)
		requireNextStatus(t, stream, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	})

	t.Run("should stream SERVICE_UNKNOWN to the watcher of an unknown service", func(t *testing.T) {
		client := startHealthServer(t, grpcserver.NewHealthHandler(resource.NewList()))

		stream := watchHealth(t, client, "unknown-service")

		requireNextStatus(t, stream, grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN)
	})

	t.Run("should end the stream when the watcher leaves", func(t *testing.T) {
		client := startHealthServer(t, grpcserver.NewHealthHandler(resource.NewList()))
		watchCtx, cancel := context.WithCancel(ctx)
		stream, err := client.Watch(watchCtx, &grpc_health_v1.HealthCheckRequest{})
		require.NoError(t, err)
		requireNextStatus(t, stream, grpc_health_v1.HealthCheckResponse_SERVING)

		cancel()

		_, err = stream.Recv()
		assert.Equal(t, codes.Canceled, status.Code(err))
	})

	t.Run("should end the stream when the status cannot be sent", func(t *testing.T) {
		stream := grpcfixture.NewFakeHealthWatchStream(ctx).FailSendWith(errors.New("send failed"))

		err := grpcserver.NewHealthHandler(resource.NewList()).Watch(&grpc_health_v1.HealthCheckRequest{}, stream)

		assert.Equal(t, codes.Canceled, status.Code(err))
	})
}
