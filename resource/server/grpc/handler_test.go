package grpc_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	grpcserver "github.com/grandper/go-athanor/resource/server/grpc"
	"github.com/grandper/go-athanor/resource/server/grpc/fixture"
)

func TestHandler(t *testing.T) {
	t.Run("should register its services on the gRPC server", func(t *testing.T) {
		var handler grpcserver.Handler = fixture.NewFakeHandler().WithRegisterFunc(func(server *grpc.Server) error {
			grpc_health_v1.RegisterHealthServer(server, health.NewServer())
			return nil
		})

		server := grpc.NewServer()
		defer server.Stop()
		require.NoError(t, handler.Register(server))

		assert.Contains(t, server.GetServiceInfo(), "grpc.health.v1.Health")
	})
}
