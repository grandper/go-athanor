package fixture_test

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource/server/nats/fixture"
)

func TestRunEmbeddedServer(t *testing.T) {
	t.Run("should start a server that accepts connections", func(t *testing.T) {
		server := fixture.RunEmbeddedServer(t)

		nc, err := nats.Connect(server.ClientURL())

		require.NoError(t, err)
		defer nc.Close()
		assert.True(t, nc.IsConnected())
	})

	t.Run("should shut the server down when the test ends", func(t *testing.T) {
		var clientURL string
		t.Run("test using the server", func(t *testing.T) {
			clientURL = fixture.RunEmbeddedServer(t).ClientURL()
		})

		_, err := nats.Connect(clientURL, nats.Timeout(time.Second))

		assert.Error(t, err, "the server must be stopped once the test that started it ends")
	})
}

func TestConnectToEmbeddedServer(t *testing.T) {
	t.Run("should return a connection to a running server", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)

		assert.True(t, nc.IsConnected())
	})

	t.Run("should close the connection when the test ends", func(t *testing.T) {
		var nc *nats.Conn
		t.Run("test using the connection", func(t *testing.T) {
			nc = fixture.ConnectToEmbeddedServer(t)
		})

		assert.True(t, nc.IsClosed())
	})
}
