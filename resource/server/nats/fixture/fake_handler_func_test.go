package fixture_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	natsserver "github.com/grandper/go-athanor/resource/server/nats"
	"github.com/grandper/go-athanor/resource/server/nats/fixture"
)

func TestFakeHandlerFunc(t *testing.T) {
	publish := func(*nats.Msg) error { return nil }

	t.Run("should succeed and record the requests it handled", func(t *testing.T) {
		handler := fixture.NewFakeHandlerFunc()
		first := &natsserver.Request{Context: context.Background(), Msg: &nats.Msg{Subject: "orders.created"}}
		second := &natsserver.Request{Context: context.Background(), Msg: &nats.Msg{Subject: "orders.deleted"}}

		require.NoError(t, handler.Handle(first, publish))
		require.NoError(t, handler.Handle(second, publish))

		assert.Equal(t, []*natsserver.Request{first, second}, handler.Requests())
		assert.Equal(t, 2, handler.CallCount())
	})

	t.Run("should record nothing before it is called", func(t *testing.T) {
		handler := fixture.NewFakeHandlerFunc()

		assert.Empty(t, handler.Requests())
		assert.Zero(t, handler.CallCount())
	})

	t.Run("fails with the configured error", func(t *testing.T) {
		errHandle := errors.New("boom")
		handler := fixture.NewFakeHandlerFunc().FailWith(errHandle)

		err := handler.Handle(&natsserver.Request{}, publish)

		require.ErrorIs(t, err, errHandle)
		assert.Equal(t, 1, handler.CallCount())
	})

	t.Run("should reply to the request when a reply is configured", func(t *testing.T) {
		handler := fixture.NewFakeHandlerFunc().ReplyWith([]byte("response"))
		var published []*nats.Msg
		record := func(msg *nats.Msg) error { published = append(published, msg); return nil }

		err := handler.Handle(&natsserver.Request{Msg: &nats.Msg{Subject: "orders.get", Reply: "inbox"}}, record)

		require.NoError(t, err)
		require.Len(t, published, 1)
		assert.Equal(t, "inbox", published[0].Subject)
		assert.Equal(t, []byte("response"), published[0].Data)
	})

	t.Run("fails when the configured reply cannot be published", func(t *testing.T) {
		errPublish := errors.New("publish failed")
		handler := fixture.NewFakeHandlerFunc().ReplyWith([]byte("response"))

		err := handler.Handle(&natsserver.Request{Msg: &nats.Msg{Reply: "inbox"}},
			func(*nats.Msg) error { return errPublish })

		require.ErrorIs(t, err, errPublish)
	})

	t.Run("should wait for the requests handled on another goroutine", func(t *testing.T) {
		handler := fixture.NewFakeHandlerFunc()
		request := &natsserver.Request{Msg: &nats.Msg{Subject: "orders.created"}}
		go func() {
			time.Sleep(10 * time.Millisecond)
			assert.NoError(t, handler.Handle(request, publish))
		}()

		requests := handler.WaitForRequests(t, 1)

		assert.Equal(t, []*natsserver.Request{request}, requests)
	})

	t.Run("should be usable as a natsserver.HandlerFunc", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)
		handler := fixture.NewFakeHandlerFunc()
		require.NoError(t, server.HandleFunc("orders.created", "", handler.Handle))
		require.NoError(t, server.ListenAndServe())

		require.NoError(t, nc.Publish("orders.created", nil))

		handler.WaitForRequests(t, 1)
		assert.Equal(t, 1, handler.CallCount())
	})
}
