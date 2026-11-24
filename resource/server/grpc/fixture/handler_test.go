package fixture_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/grandper/go-athanor/resource/server/grpc/fixture"
)

func TestFakeHandler(t *testing.T) {
	t.Run("should record that it was registered", func(t *testing.T) {
		handler := fixture.NewFakeHandler()
		assert.False(t, handler.WasRegistered())

		server := grpc.NewServer()
		defer server.Stop()
		require.NoError(t, handler.Register(server))

		assert.True(t, handler.WasRegistered())
	})

	t.Run("should delegate the registration to the configured function", func(t *testing.T) {
		var registeredServer *grpc.Server
		handler := fixture.NewFakeHandler().WithRegisterFunc(func(server *grpc.Server) error {
			registeredServer = server
			return nil
		})

		server := grpc.NewServer()
		defer server.Stop()
		require.NoError(t, handler.Register(server))

		assert.Same(t, server, registeredServer)
	})

	t.Run("fails the registration with the configured error", func(t *testing.T) {
		errRegister := errors.New("registration failed")
		handler := fixture.NewFakeHandler().FailRegisterWith(errRegister)

		server := grpc.NewServer()
		defer server.Stop()
		err := handler.Register(server)

		require.ErrorIs(t, err, errRegister)
		assert.True(t, handler.WasRegistered())
	})
}

func TestPanickingHandler(t *testing.T) {
	t.Run("should register a health service on the server", func(t *testing.T) {
		server := grpc.NewServer()

		require.NoError(t, fixture.NewPanickingHandler("boom").Register(server))

		assert.Contains(t, server.GetServiceInfo(), "grpc.health.v1.Health")
	})

	t.Run("should panic with the given value on Check", func(t *testing.T) {
		handler := fixture.NewPanickingHandler("the order has no line")

		assert.PanicsWithValue(t, "the order has no line", func() {
			_, _ = handler.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
		})
	})

	t.Run("should panic with the given value on Watch", func(t *testing.T) {
		handler := fixture.NewPanickingHandler("the order has no line")
		stream := fixture.NewFakeHealthWatchStream(context.Background())

		assert.PanicsWithValue(t, "the order has no line", func() {
			_ = handler.Watch(&grpc_health_v1.HealthCheckRequest{}, stream)
		})
	})
}
