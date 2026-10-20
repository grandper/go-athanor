package fixture

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// SendMulticastDatagram sends payload in one datagram to the multicast group through the interface, from a
// socket of its own. The sending socket loops the datagram back, so a listener joined on the interface gets it.
func SendMulticastDatagram(t *testing.T, iface *net.Interface, group *net.UDPAddr, payload []byte) {
	t.Helper()

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: IPv4Of(t, iface)})
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()

	rawConn, err := conn.SyscallConn()
	require.NoError(t, err)
	var setErr error
	require.NoError(t, rawConn.Control(func(fd uintptr) {
		setErr = setMulticastInterface(fd, [4]byte(IPv4Of(t, iface)))
	}))
	require.NoError(t, setErr, "failed to route multicast through %s", iface.Name)

	_, err = conn.WriteToUDP(payload, group)
	require.NoError(t, err)
}
