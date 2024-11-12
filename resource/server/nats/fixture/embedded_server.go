package fixture

import (
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

	options := natstest.DefaultTestOptions
	options.Port = server.RANDOM_PORT
	natsServer := natstest.RunServer(&options)
	t.Cleanup(natsServer.Shutdown)
	require.True(t, natsServer.ReadyForConnections(readyTimeout), "the embedded NATS server never became ready")

	return natsServer
}

// ConnectToEmbeddedServer starts an in-process NATS server and returns a connection to it.
// The connection is closed, then the server stopped, when the test ends.
func ConnectToEmbeddedServer(t *testing.T) *nats.Conn {
	t.Helper()

	natsServer := RunEmbeddedServer(t)

	nc, err := nats.Connect(natsServer.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	return nc
}
