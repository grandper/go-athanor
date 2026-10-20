package fixture_test

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grandper/go-athanor/resource/server/udp/fixture"
)

func TestMulticastInterface(t *testing.T) {
	t.Run("should return an interface that is up, multicast capable, and has an IPv4 address", func(t *testing.T) {
		iface := fixture.MulticastInterface(t)

		assert.NotZero(t, iface.Flags&net.FlagUp, "the interface %s is down", iface.Name)
		assert.NotZero(t, iface.Flags&net.FlagMulticast, "the interface %s has no multicast", iface.Name)
		assert.NotNil(t, fixture.IPv4Of(t, iface))
	})
}
