package nats_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	natsserver "github.com/grandper/go-athanor/resource/server/nats"
	"github.com/grandper/go-athanor/resource/server/nats/fixture"
)

func TestHandlerFunc(t *testing.T) {
	t.Run("should dispatch a message to the registered handler, which replies through publish", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		handler := fixture.NewFakeHandlerFunc().ReplyWith([]byte("response"))
		require.NoError(t, server.HandleFunc("orders.created", "orders", handler.Handle))
		require.NoError(t, server.ListenAndServe())

		reply, err := nc.Request("orders.created", []byte("order"), requestTimeout)

		require.NoError(t, err)
		assert.Equal(t, []byte("response"), reply.Data)
		requests := handler.WaitForRequests(t, 1)
		assert.Equal(t, []byte("order"), requests[0].Msg.Data)
	})

	t.Run("should report handler errors to the OnHandlerFailed hooks", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		recorder := fixture.NewHookRecorder()
		server := natsserver.NewServer(nc).WithHooks(recorder.HandlerFailedHook())
		errHandle := errors.New("boom")
		handle := fixture.NewFakeHandlerFunc().FailWith(errHandle).Handle
		require.NoError(t, server.HandleFunc("orders.created", "", handle))
		require.NoError(t, server.ListenAndServe())

		require.NoError(t, nc.Publish("orders.created", nil))

		assert.Equal(t, []string{"handler failed: orders.created: boom"}, recorder.WaitForEvents(t, 1))
		assert.ErrorIs(t, recorder.HandlerFailures()[0].Err, errHandle)
	})
}
