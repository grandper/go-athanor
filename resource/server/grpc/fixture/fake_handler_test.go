package fixture_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

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
