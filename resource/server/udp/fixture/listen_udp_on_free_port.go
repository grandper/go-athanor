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
