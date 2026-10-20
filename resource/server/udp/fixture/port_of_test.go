package fixture_test

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grandper/go-athanor/resource/server/udp/fixture"
)

func TestPortOf(t *testing.T) {
	t.Run("should return the port of a UDP address", func(t *testing.T) {
		address := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5555}

		assert.Equal(t, 5555, fixture.PortOf(t, address))
	})
}
