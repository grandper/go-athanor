package nats_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	natsserver "github.com/grandper/go-athanor/resource/server/nats"
	"github.com/grandper/go-athanor/resource/server/nats/fixture"
)

func TestHandler(t *testing.T) {
	t.Run("should register its subscriptions on the NATS server", func(t *testing.T) {
		var handler natsserver.Handler = fixture.NewFakeHandler().
			WithHandleFunc("orders.created", "orders", fixture.NewFakeHandlerFunc().Handle)

		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		require.NoError(t, handler.Register(server))
		require.NoError(t, server.ListenAndServe())

		assert.Equal(t, 1, nc.NumSubscriptions())
	})
}
