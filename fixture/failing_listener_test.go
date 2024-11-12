package fixture_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/fixture"
)

func TestFailingListener(t *testing.T) {
	errAccept := errors.New("accept failed")

	t.Run("fails to accept with the configured error", func(t *testing.T) {
		listener := fixture.NewFailingListener(errAccept)

		conn, err := listener.Accept()

		require.ErrorIs(t, err, errAccept)
		assert.Nil(t, conn)
	})

	t.Run("should record that it was closed", func(t *testing.T) {
		listener := fixture.NewFailingListener(errAccept)
		assert.False(t, listener.WasClosed())

		err := listener.Close()

		require.NoError(t, err)
		assert.True(t, listener.WasClosed())
	})

	t.Run("should report a TCP address", func(t *testing.T) {
		listener := fixture.NewFailingListener(errAccept)

		assert.Equal(t, "tcp", listener.Addr().Network())
	})
}
