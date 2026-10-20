package udp_test

import (
	"context"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource/server/udp"
)

func TestHandlerFunc(t *testing.T) {
	ctx := context.Background()

	t.Run("should delegate to the function with the datagram", func(t *testing.T) {
		datagram := udp.Datagram{
			Payload: []byte("hello"),
			Source:  netip.MustParseAddrPort("127.0.0.1:5555"),
		}
		var received udp.Datagram
		handler := udp.HandlerFunc(func(_ context.Context, d udp.Datagram) error {
			received = d
			return nil
		})

		err := handler.Handle(ctx, datagram)

		require.NoError(t, err)
		assert.Equal(t, datagram, received)
	})

	t.Run("fails when the function fails", func(t *testing.T) {
		handler := udp.HandlerFunc(func(context.Context, udp.Datagram) error { return assert.AnError })

		err := handler.Handle(ctx, udp.Datagram{})

		require.ErrorIs(t, err, assert.AnError)
	})

	t.Run("implements the Handler interface", func(t *testing.T) {
		var handler udp.Handler = udp.HandlerFunc(
			func(context.Context, udp.Datagram) error { return nil },
		)

		require.NoError(t, handler.Handle(ctx, udp.Datagram{}))
	})
}
