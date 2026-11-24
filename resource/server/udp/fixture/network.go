package fixture

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// loopbackAddress is the IPv4 loopback address the fixtures bind.
const loopbackAddress = "127.0.0.1"

// ListenUDPOnFreePort occupies a loopback UDP port picked by the operating system and returns the socket,
// closed when the test ends. The port cannot be taken by anyone else meanwhile.
func ListenUDPOnFreePort(t *testing.T) *net.UDPConn {
	t.Helper()

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(loopbackAddress)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// PortOf returns the port of the UDP address.
func PortOf(t *testing.T, address net.Addr) int {
	t.Helper()

	udpAddress, ok := address.(*net.UDPAddr)
	require.True(t, ok, "%s is not a UDP address", address)
	return udpAddress.Port
}

// SendDatagram sends payload in one datagram to the UDP address, from a socket of its own.
func SendDatagram(t *testing.T, address net.Addr, payload []byte) {
	t.Helper()

	target, err := net.ResolveUDPAddr("udp4", address.String())
	require.NoError(t, err)

	conn, err := net.DialUDP("udp4", nil, target)
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()

	_, err = conn.Write(payload)
	require.NoError(t, err)
}

// SendDatagramFrom sends payload in one datagram to the UDP address, from the socket conn, so that the receiver
// sees conn as the source of the datagram.
func SendDatagramFrom(t *testing.T, conn *net.UDPConn, address net.Addr, payload []byte) {
	t.Helper()

	target, err := net.ResolveUDPAddr("udp4", address.String())
	require.NoError(t, err)
	_, err = conn.WriteToUDP(payload, target)
	require.NoError(t, err)
}
