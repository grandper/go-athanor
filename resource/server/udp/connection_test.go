package udp_test

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource/server/udp"
)

func TestConnection(t *testing.T) {
	t.Run("is implemented by a UDP connection", func(t *testing.T) {
		udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		require.NoError(t, err)
		defer func() { require.NoError(t, udpConn.Close()) }()

		var conn udp.Connection = udpConn

		assert.Equal(t, udpConn.LocalAddr(), conn.LocalAddr())
	})
}
