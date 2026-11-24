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

	t.Run("should block, once it recorded the request, until released", func(t *testing.T) {
		release := make(chan struct{})
		handler := fixture.NewFakeHandlerFunc().BlockUntil(release)
		returned := make(chan error, 1)
		go func() { returned <- handler.Handle(&natsserver.Request{}, publish) }()

		handler.WaitForRequests(t, 1)
		assert.Empty(t, returned, "the handler must block until released")
		close(release)

		assert.Eventually(t, func() bool { return len(returned) == 1 }, time.Second, time.Millisecond)
		assert.NoError(t, <-returned)
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

func TestFakePublishFunc(t *testing.T) {
	t.Run("should record the published messages in order", func(t *testing.T) {
		publisher := fixture.NewFakePublishFunc()
		first := &nats.Msg{Subject: "inbox", Data: []byte("first")}
		second := &nats.Msg{Subject: "inbox", Data: []byte("second")}

		require.NoError(t, publisher.Publish(first))
		require.NoError(t, publisher.Publish(second))

		assert.Equal(t, []*nats.Msg{first, second}, publisher.Published())
	})

	t.Run("should record nothing before it is called", func(t *testing.T) {
		publisher := fixture.NewFakePublishFunc()

		assert.Empty(t, publisher.Published())
	})

	t.Run("should return a snapshot of the published messages", func(t *testing.T) {
		publisher := fixture.NewFakePublishFunc()
		require.NoError(t, publisher.Publish(&nats.Msg{Subject: "inbox"}))

		published := publisher.Published()
		published[0] = nil

		assert.NotNil(t, publisher.Published()[0])
	})

	t.Run("fails with the configured error without recording the message", func(t *testing.T) {
		errPublish := errors.New("publish failed")
		publisher := fixture.NewFakePublishFunc().FailWith(errPublish)

		err := publisher.Publish(&nats.Msg{Subject: "inbox"})

		require.ErrorIs(t, err, errPublish)
		assert.Empty(t, publisher.Published())
	})

	t.Run("should be usable as a natsserver.PublishFunc", func(t *testing.T) {
		publisher := fixture.NewFakePublishFunc()
		handler := fixture.NewFakeHandlerFunc().ReplyWith([]byte("response"))

		err := handler.Handle(&natsserver.Request{Msg: &nats.Msg{Subject: "orders.get", Reply: "inbox"}},
			publisher.Publish)

		require.NoError(t, err)
		require.Len(t, publisher.Published(), 1)
		assert.Equal(t, "inbox", publisher.Published()[0].Subject)
	})
}
