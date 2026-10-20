package fixture_test

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource/server/udp"
	"github.com/grandper/go-athanor/resource/server/udp/fixture"
)

const waitTimeout = time.Second

func TestRecordingHandler(t *testing.T) {
	ctx := context.Background()
	datagram := udp.Datagram{
		Payload: []byte("hello"),
		Source:  netip.MustParseAddrPort("127.0.0.1:5555"),
	}

	t.Run("should record the datagrams it handles", func(t *testing.T) {
		handler := fixture.NewRecordingHandler()
		assert.Empty(t, handler.Datagrams())
		assert.Zero(t, handler.Calls())

		err := handler.Handle(ctx, datagram)

		require.NoError(t, err)
		assert.Equal(t, []udp.Datagram{datagram}, handler.Datagrams())
		assert.Equal(t, 1, handler.Calls())
	})

	t.Run("should copy the payload, so a reused buffer does not alter the record", func(t *testing.T) {
		handler := fixture.NewRecordingHandler()
		buffer := []byte("hello")

		require.NoError(t, handler.Handle(ctx, udp.Datagram{Payload: buffer}))
		copy(buffer, "bye!!")

		assert.Equal(t, []byte("hello"), handler.Datagrams()[0].Payload)
	})

	t.Run("should hand the next datagram over to whoever waits for it", func(t *testing.T) {
		handler := fixture.NewRecordingHandler()
		go func() { _ = handler.Handle(ctx, datagram) }()

		received := handler.WaitForDatagram(t, waitTimeout)

		assert.Equal(t, datagram, received)
	})

	t.Run("fails with the configured error, still recording the datagram", func(t *testing.T) {
		handler := fixture.NewRecordingHandler().FailWith(assert.AnError)

		err := handler.Handle(ctx, datagram)

		require.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, 1, handler.Calls())
	})

	t.Run("should delegate to the configured function, still recording the datagram", func(t *testing.T) {
		var seen udp.Datagram
		handler := fixture.NewRecordingHandler().WithHandleFunc(func(_ context.Context, d udp.Datagram) error {
			seen = d
			return assert.AnError
		})

		err := handler.Handle(ctx, datagram)

		require.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, datagram, seen)
		assert.Equal(t, 1, handler.Calls())
	})
}
