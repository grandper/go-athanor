package nats_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	natsserver "github.com/grandper/go-athanor/resource/server/nats"
	"github.com/grandper/go-athanor/resource/server/nats/fixture"
)

func TestRequest(t *testing.T) {
	t.Run("should give handlers a non-nil context and the received message", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		handler := fixture.NewFakeHandlerFunc()
		require.NoError(t, server.HandleFunc("orders.created", "", handler.Handle))
		require.NoError(t, server.ListenAndServe())

		require.NoError(t, nc.Publish("orders.created", []byte("order")))

		request := handler.WaitForRequests(t, 1)[0]
		assert.NotNil(t, request.Context)
		assert.Equal(t, "orders.created", request.Msg.Subject)
		assert.Equal(t, []byte("order"), request.Msg.Data)
	})
}
