package fixture

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

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
