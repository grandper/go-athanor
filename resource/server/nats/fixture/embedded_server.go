package fixture

import (
	"net"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

const readyTimeout = 2 * time.Second

// RunEmbeddedServer starts an in-process NATS server on a random port and stops it when the test ends.
func RunEmbeddedServer(t *testing.T) *server.Server {
	t.Helper()

	return RunEmbeddedServerOnPort(t, server.RANDOM_PORT)
}

// RunEmbeddedServerOnPort starts an in-process NATS server on port and stops it when the test ends.
// Together with StopEmbeddedServer, it restarts a server on the port its clients reconnect to.
func RunEmbeddedServerOnPort(t *testing.T, port int) *server.Server {
	t.Helper()

	options := natstest.DefaultTestOptions
	options.Port = port
	natsServer := natstest.RunServer(&options)
	t.Cleanup(natsServer.Shutdown)
	require.True(t, natsServer.ReadyForConnections(readyTimeout), "the embedded NATS server never became ready")

	return natsServer
}

// StopEmbeddedServer stops the in-process NATS server natsServer, waits until it is down, and returns the port
// it listened on, so that RunEmbeddedServerOnPort can start a new server in its place.
func StopEmbeddedServer(t *testing.T, natsServer *server.Server) int {
	t.Helper()

	address, ok := natsServer.Addr().(*net.TCPAddr)
	require.True(t, ok, "the embedded NATS server does not listen on a TCP address")
	natsServer.Shutdown()
	natsServer.WaitForShutdown()

	return address.Port
}

// ConnectToEmbeddedServer starts an in-process NATS server and returns a connection to it, made with options.
// The connection is closed, then the server stopped, when the test ends.
func ConnectToEmbeddedServer(t *testing.T, options ...nats.Option) *nats.Conn {
	t.Helper()

	return ConnectTo(t, RunEmbeddedServer(t), options...)
}

// ConnectTo returns a connection to the in-process NATS server natsServer, made with options.
// The connection is closed when the test ends.
func ConnectTo(t *testing.T, natsServer *server.Server, options ...nats.Option) *nats.Conn {
	t.Helper()

	nc, err := nats.Connect(natsServer.ClientURL(), options...)
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	return nc
}

// reconnectWait is the short pause between two reconnection attempts of ReconnectQuickly.
const reconnectWait = 10 * time.Millisecond

// ReconnectQuickly returns a nats.Connect option making the client reconnect forever, with a short pause and no
// jitter between attempts, so that a test sees the connection come back as soon as the server restarts.
func ReconnectQuickly() nats.Option {
	return func(options *nats.Options) error {
		options.AllowReconnect = true
		options.MaxReconnect = -1
		options.ReconnectWait = reconnectWait
		options.ReconnectJitter = 0
		options.ReconnectJitterTLS = 0
		return nil
	}
}
