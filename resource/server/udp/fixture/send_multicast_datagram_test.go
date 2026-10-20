package fixture_test

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource/server/udp/fixture"
)

func TestSendMulticastDatagram(t *testing.T) {
	t.Run("should deliver the payload to the group joined on the interface", func(t *testing.T) {
		iface := fixture.MulticastInterface(t)
		group := &net.UDPAddr{IP: net.ParseIP(fixture.MulticastGroup)}
		conn, err := net.ListenMulticastUDP("udp4", iface, group)
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()
		group.Port = fixture.PortOf(t, conn.LocalAddr())

		fixture.SendMulticastDatagram(t, iface, group, []byte("hello"))

		require.NoError(t, conn.SetReadDeadline(time.Now().Add(waitTimeout)))
		buffer := make([]byte, 64)
		n, _, err := conn.ReadFromUDP(buffer)
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), buffer[:n])
	})
}
