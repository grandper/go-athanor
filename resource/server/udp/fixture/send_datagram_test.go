package fixture_test

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource/server/udp/fixture"
)

func TestSendDatagram(t *testing.T) {
	t.Run("should deliver the payload to the address", func(t *testing.T) {
		conn := fixture.ListenUDPOnFreePort(t)

		fixture.SendDatagram(t, conn.LocalAddr(), []byte("hello"))

		require.NoError(t, conn.SetReadDeadline(time.Now().Add(waitTimeout)))
		buffer := make([]byte, 64)
		n, source, err := conn.ReadFromUDP(buffer)
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), buffer[:n])
		assert.True(t, source.IP.Equal(net.IPv4(127, 0, 0, 1)), "unexpected source %s", source)
	})
}
