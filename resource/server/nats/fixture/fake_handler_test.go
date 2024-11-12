package fixture_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	natsserver "github.com/grandper/go-athanor/resource/server/nats"
	"github.com/grandper/go-athanor/resource/server/nats/fixture"
)

func TestFakeHandler(t *testing.T) {
	t.Run("should record that it was registered", func(t *testing.T) {
		handler := fixture.NewFakeHandler()
		assert.False(t, handler.WasRegistered())

		require.NoError(t, handler.Register(natsserver.NewServer(fixture.ConnectToEmbeddedServer(t))))

		assert.True(t, handler.WasRegistered())
	})

	t.Run("should delegate the registration to the configured function", func(t *testing.T) {
		var registeredServer *natsserver.Server
		handler := fixture.NewFakeHandler().WithRegisterFunc(func(server *natsserver.Server) error {
			registeredServer = server
			return nil
		})

		server := natsserver.NewServer(fixture.ConnectToEmbeddedServer(t))
		require.NoError(t, handler.Register(server))

		assert.Same(t, server, registeredServer)
	})

	t.Run("should register the configured handler functions on the server", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		handle := fixture.NewFakeHandlerFunc()
		handler := fixture.NewFakeHandler().
			WithHandleFunc("orders.created", "orders", handle.Handle).
			WithHandleFunc("orders.deleted", "", handle.Handle)
		server := natsserver.NewServer(nc, handler)

		require.NoError(t, server.Init(context.Background()))

		assert.Equal(t, 2, nc.NumSubscriptions())
		require.NoError(t, nc.Publish("orders.created", nil))
		require.NoError(t, nc.Publish("orders.deleted", nil))
		handle.WaitForRequests(t, 2)
	})

	t.Run("fails the registration when a configured handler function is refused", func(t *testing.T) {
		handler := fixture.NewFakeHandler().WithHandleFunc("", "", fixture.NewFakeHandlerFunc().Handle)

		err := handler.Register(natsserver.NewServer(fixture.ConnectToEmbeddedServer(t)))

		require.ErrorIs(t, err, natsserver.ErrEmptySubject)
	})

	t.Run("fails the registration with the configured error", func(t *testing.T) {
		errRegister := errors.New("registration failed")
		handler := fixture.NewFakeHandler().FailRegisterWith(errRegister)

		err := handler.Register(natsserver.NewServer(fixture.ConnectToEmbeddedServer(t)))

		require.ErrorIs(t, err, errRegister)
		assert.True(t, handler.WasRegistered())
	})
}
