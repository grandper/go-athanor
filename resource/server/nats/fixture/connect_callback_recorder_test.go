package fixture_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource/server/nats/fixture"
)

func TestConnectCallbackRecorder(t *testing.T) {
	t.Run("should record nothing before the client calls back", func(t *testing.T) {
		callbacks := fixture.NewConnectCallbackRecorder()

		fixture.ConnectToEmbeddedServer(t, callbacks.Options()...)

		assert.Empty(t, callbacks.Events())
	})

	t.Run("should record the disconnect, then the closed callback, when the connection is closed", func(t *testing.T) {
		callbacks := fixture.NewConnectCallbackRecorder()
		nc := fixture.ConnectToEmbeddedServer(t, callbacks.Options()...)

		nc.Close()

		assert.Equal(t, []string{"disconnected: <nil>", "closed"}, callbacks.WaitForEvents(t, 2))
	})

	t.Run("should record the disconnect and the reconnect callbacks when the server restarts", func(t *testing.T) {
		callbacks := fixture.NewConnectCallbackRecorder()
		natsServer := fixture.RunEmbeddedServer(t)
		fixture.ConnectTo(t, natsServer, append(callbacks.Options(), fixture.ReconnectQuickly())...)

		port := fixture.StopEmbeddedServer(t, natsServer)
		lost := callbacks.WaitForEvents(t, 1)
		fixture.RunEmbeddedServerOnPort(t, port)

		events := callbacks.WaitForEvents(t, 2)
		require.Len(t, lost, 1)
		assert.True(t, strings.HasPrefix(lost[0], "disconnected: "), "unexpected event %q", lost[0])
		assert.Equal(t, "reconnected", events[1])
	})

	t.Run("should return a snapshot of the events", func(t *testing.T) {
		callbacks := fixture.NewConnectCallbackRecorder()
		fixture.ConnectToEmbeddedServer(t, callbacks.Options()...).Close()
		events := callbacks.WaitForEvents(t, 2)

		events[0] = "changed"

		assert.Equal(t, []string{"disconnected: <nil>", "closed"}, callbacks.Events())
	})
}
