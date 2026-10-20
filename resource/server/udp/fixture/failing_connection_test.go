package fixture_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource/server/udp/fixture"
)

func TestFailingConnection(t *testing.T) {
	t.Run("fails every read with the configured error", func(t *testing.T) {
		conn := fixture.NewFailingConnection(assert.AnError)

		n, source, err := conn.ReadFromUDPAddrPort(make([]byte, 16))

		require.ErrorIs(t, err, assert.AnError)
		assert.Zero(t, n)
		assert.False(t, source.IsValid())
	})

	t.Run("should record that it was closed", func(t *testing.T) {
		conn := fixture.NewFailingConnection(assert.AnError)
		assert.False(t, conn.WasClosed())

		require.NoError(t, conn.Close())

		assert.True(t, conn.WasClosed())
	})

	t.Run("should accept a read deadline", func(t *testing.T) {
		conn := fixture.NewFailingConnection(assert.AnError)

		require.NoError(t, conn.SetReadDeadline(time.Now()))
	})

	t.Run("should have a local address", func(t *testing.T) {
		conn := fixture.NewFailingConnection(assert.AnError)

		assert.NotNil(t, conn.LocalAddr())
	})
}
