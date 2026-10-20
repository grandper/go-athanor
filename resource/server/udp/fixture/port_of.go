package fixture

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// PortOf returns the port of the UDP address.
func PortOf(t *testing.T, address net.Addr) int {
	t.Helper()

	udpAddress, ok := address.(*net.UDPAddr)
	require.True(t, ok, "%s is not a UDP address", address)
	return udpAddress.Port
}
