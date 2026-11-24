package fixture_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	athanorfixture "github.com/grandper/go-athanor/fixture"
	"github.com/grandper/go-athanor/resource/server/nats/fixture"
)

const (
	waitTimeout  = 2 * time.Second
	pollInterval = 10 * time.Millisecond
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

func TestRunEmbeddedServerOnPort(t *testing.T) {
	t.Run("should start a server on the given port that accepts connections", func(t *testing.T) {
		port := athanorfixture.GetFreePort(t)

		server := fixture.RunEmbeddedServerOnPort(t, port)

		assert.Equal(t, fmt.Sprintf("nats://127.0.0.1:%d", port), server.ClientURL())
		nc, err := nats.Connect(server.ClientURL())
		require.NoError(t, err)
		defer nc.Close()
		assert.True(t, nc.IsConnected())
	})

	t.Run("should shut the server down when the test ends", func(t *testing.T) {
		port := athanorfixture.GetFreePort(t)
		t.Run("test using the server", func(t *testing.T) {
			fixture.RunEmbeddedServerOnPort(t, port)
		})

		_, err := nats.Connect(fmt.Sprintf("nats://127.0.0.1:%d", port), nats.Timeout(time.Second))

		assert.Error(t, err, "the server must be stopped once the test that started it ends")
	})
}

func TestStopEmbeddedServer(t *testing.T) {
	t.Run("should stop the server and return the port it listened on", func(t *testing.T) {
		server := fixture.RunEmbeddedServer(t)
		clientURL := server.ClientURL()

		port := fixture.StopEmbeddedServer(t, server)

		assert.Equal(t, fmt.Sprintf("nats://127.0.0.1:%d", port), clientURL)
		assert.False(t, server.Running())
		_, err := nats.Connect(clientURL, nats.Timeout(time.Second))
		assert.Error(t, err, "the stopped server must refuse connections")
	})

	t.Run("should free the port for a new server", func(t *testing.T) {
		port := fixture.StopEmbeddedServer(t, fixture.RunEmbeddedServer(t))

		server := fixture.RunEmbeddedServerOnPort(t, port)

		assert.True(t, server.Running())
	})
}

func TestConnectTo(t *testing.T) {
	t.Run("should return a connection to the server, made with the options", func(t *testing.T) {
		server := fixture.RunEmbeddedServer(t)

		nc := fixture.ConnectTo(t, server, nats.Name("orders"))

		assert.True(t, nc.IsConnected())
		assert.Equal(t, server.ClientURL(), nc.ConnectedUrl())
		assert.Equal(t, "orders", nc.Opts.Name)
	})

	t.Run("should close the connection when the test ends", func(t *testing.T) {
		server := fixture.RunEmbeddedServer(t)
		var nc *nats.Conn
		t.Run("test using the connection", func(t *testing.T) {
			nc = fixture.ConnectTo(t, server)
		})

		assert.True(t, nc.IsClosed())
	})
}

func TestConnectToEmbeddedServer(t *testing.T) {
	t.Run("should make the connection with the options", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t, nats.Name("orders"))

		assert.Equal(t, "orders", nc.Opts.Name)
	})

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

func TestReconnectQuickly(t *testing.T) {
	t.Run("should make the client reconnect forever, quickly and without jitter", func(t *testing.T) {
		options := nats.GetDefaultOptions()

		require.NoError(t, fixture.ReconnectQuickly()(&options))

		assert.True(t, options.AllowReconnect)
		assert.Equal(t, -1, options.MaxReconnect)
		assert.Less(t, options.ReconnectWait, nats.DefaultReconnectWait)
		assert.Zero(t, options.ReconnectJitter)
		assert.Zero(t, options.ReconnectJitterTLS)
	})

	t.Run("should let a connection come back once the server restarts on the same port", func(t *testing.T) {
		natsServer := fixture.RunEmbeddedServer(t)
		nc := fixture.ConnectTo(t, natsServer, fixture.ReconnectQuickly())
		port := fixture.StopEmbeddedServer(t, natsServer)
		require.Eventually(t, nc.IsReconnecting, waitTimeout, pollInterval)

		fixture.RunEmbeddedServerOnPort(t, port)

		assert.Eventually(t, nc.IsConnected, waitTimeout, pollInterval)
	})
}
