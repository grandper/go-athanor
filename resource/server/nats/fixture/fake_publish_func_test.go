package fixture_test

import (
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	natsserver "github.com/grandper/go-athanor/resource/server/nats"
	"github.com/grandper/go-athanor/resource/server/nats/fixture"
)

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
