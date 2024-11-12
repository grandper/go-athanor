package fixture_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
