package fixture_test

import (
	"net"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource/server/udp/fixture"
)

func TestListenUDPOnFreePort(t *testing.T) {
	t.Run("should bind a loopback UDP port", func(t *testing.T) {
		conn := fixture.ListenUDPOnFreePort(t)

		address, ok := conn.LocalAddr().(*net.UDPAddr)
		require.True(t, ok)
		assert.True(t, address.IP.IsLoopback(), "not a loopback address: %s", address)
		assert.Positive(t, address.Port)
	})

	t.Run("should keep the port, so binding it again fails", func(t *testing.T) {
		conn := fixture.ListenUDPOnFreePort(t)

		address, err := net.ResolveUDPAddr("udp4", conn.LocalAddr().String())
		require.NoError(t, err)
		_, err = net.ListenUDP("udp4", address)

		require.ErrorIs(t, err, syscall.EADDRINUSE)
	})
}
