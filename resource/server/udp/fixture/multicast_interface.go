package fixture

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// MulticastGroup is an administratively scoped IPv4 multicast group address for tests.
const MulticastGroup = "239.255.0.1"

// MulticastInterface returns a network interface that is up, multicast capable, and has an IPv4 address,
// preferring the loopback interface. It skips the test when the host has none.
func MulticastInterface(t *testing.T) *net.Interface {
	t.Helper()

	interfaces, err := net.Interfaces()
	require.NoError(t, err)

	var candidate *net.Interface
	for i := range interfaces {
		iface := &interfaces[i]
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 || ipv4Of(iface) == nil {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			return iface
		}
		if candidate == nil {
			candidate = iface
		}
	}
	if candidate == nil {
		t.Skip("no multicast capable interface with an IPv4 address on this host")
	}
	return candidate
}

// IPv4Of returns the first IPv4 address of the interface, or fails the test when it has none.
func IPv4Of(t *testing.T, iface *net.Interface) net.IP {
	t.Helper()

	ip := ipv4Of(iface)
	require.NotNil(t, ip, "the interface %s has no IPv4 address", iface.Name)
	return ip
}

// ipv4Of returns the first IPv4 address of the interface, or nil when it has none.
func ipv4Of(iface *net.Interface) net.IP {
	addresses, err := iface.Addrs()
	if err != nil {
		return nil
	}
	for _, address := range addresses {
		if network, ok := address.(*net.IPNet); ok && network.IP.To4() != nil {
			return network.IP.To4()
		}
	}
	return nil
}
