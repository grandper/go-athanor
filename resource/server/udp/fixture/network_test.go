package fixture_test

import (
	"net"
	"syscall"
	"testing"
	"time"

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

func TestPortOf(t *testing.T) {
	t.Run("should return the port of a UDP address", func(t *testing.T) {
		address := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5555}

		assert.Equal(t, 5555, fixture.PortOf(t, address))
	})
}

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

func TestSendDatagramFrom(t *testing.T) {
	t.Run("should deliver the payload to the address, from the given socket", func(t *testing.T) {
		receiver := fixture.ListenUDPOnFreePort(t)
		sender := fixture.ListenUDPOnFreePort(t)

		fixture.SendDatagramFrom(t, sender, receiver.LocalAddr(), []byte("hello"))

		require.NoError(t, receiver.SetReadDeadline(time.Now().Add(waitTimeout)))
		buffer := make([]byte, 64)
		n, source, err := receiver.ReadFromUDP(buffer)
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), buffer[:n])
		assert.Equal(t, fixture.PortOf(t, sender.LocalAddr()), source.Port)
	})
}
