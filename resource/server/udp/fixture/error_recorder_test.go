package fixture_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource/server/udp"
	"github.com/grandper/go-athanor/resource/server/udp/fixture"
)

// serveFailingHandler starts a UDP server whose handler fails with err, hooked with hooks, and returns its address.
// The server is shut down when the test ends.
func serveFailingHandler(t *testing.T, err error, hooks ...udp.Hook) net.Addr {
	t.Helper()
	ctx := context.Background()
	config := &udp.Config{Address: "127.0.0.1:0", BufferSize: 64}
	server := udp.NewServer(config, fixture.NewRecordingHandler().FailWith(err)).WithHooks(hooks...)
	require.NoError(t, server.Init(ctx))
	t.Cleanup(func() { _ = server.Shutdown(ctx) })
	return server.LocalAddress()
}

func TestErrorRecorder(t *testing.T) {
	t.Run("should record nothing before the server reports a failure", func(t *testing.T) {
		recorder := fixture.NewErrorRecorder()

		assert.Empty(t, recorder.Events())
		assert.Empty(t, recorder.Failures())
	})

	t.Run("should return one hook per lifecycle event", func(t *testing.T) {
		recorder := fixture.NewErrorRecorder()

		assert.Len(t, recorder.Hooks(), 1)
		assert.Len(t, recorder.HooksLabeled("first"), 1)
	})

	t.Run("should record a handler failure with its payload and error", func(t *testing.T) {
		recorder := fixture.NewErrorRecorder()
		address := serveFailingHandler(t, errors.New("unknown frame type 0x7f"), recorder.Hooks()...)

		fixture.SendDatagram(t, address, []byte("frame"))

		assert.Equal(t, []string{`handler failed: "frame": unknown frame type 0x7f`},
			recorder.WaitForEvents(t, 1, waitTimeout))
	})

	t.Run("should record what the OnHandlerFailed hook received", func(t *testing.T) {
		recorder := fixture.NewErrorRecorder()
		errHandle := errors.New("boom")
		address := serveFailingHandler(t, errHandle, recorder.Hooks()...)
		sender := fixture.ListenUDPOnFreePort(t)

		fixture.SendDatagramFrom(t, sender, address, []byte("frame"))

		failures := recorder.WaitForFailures(t, 1, waitTimeout)
		require.Len(t, failures, 1)
		assert.Equal(t, []byte("frame"), failures[0].Datagram.Payload)
		assert.Equal(t, fixture.PortOf(t, sender.LocalAddr()), int(failures[0].Datagram.Source.Port()))
		assert.ErrorIs(t, failures[0].Err, errHandle)
	})

	t.Run("should record the failures in the order they happened", func(t *testing.T) {
		recorder := fixture.NewErrorRecorder()
		address := serveFailingHandler(t, errors.New("boom"), recorder.Hooks()...)

		fixture.SendDatagram(t, address, []byte("first"))
		recorder.WaitForFailures(t, 1, waitTimeout)
		fixture.SendDatagram(t, address, []byte("second"))

		failures := recorder.WaitForFailures(t, 2, waitTimeout)
		assert.Equal(t, []byte("first"), failures[0].Datagram.Payload)
		assert.Equal(t, []byte("second"), failures[1].Datagram.Payload)
	})

	t.Run("should prefix the events of labeled hooks with their label", func(t *testing.T) {
		recorder := fixture.NewErrorRecorder()
		address := serveFailingHandler(t, errors.New("boom"),
			append(recorder.HooksLabeled("first"), recorder.HooksLabeled("second")...)...)

		fixture.SendDatagram(t, address, []byte("frame"))

		assert.Equal(t, []string{
			`first: handler failed: "frame": boom`,
			`second: handler failed: "frame": boom`,
		}, recorder.WaitForEvents(t, 2, waitTimeout))
	})

	t.Run("should return a snapshot of the failures", func(t *testing.T) {
		recorder := fixture.NewErrorRecorder()
		address := serveFailingHandler(t, errors.New("boom"), recorder.Hooks()...)
		fixture.SendDatagram(t, address, []byte("frame"))
		failures := recorder.WaitForFailures(t, 1, waitTimeout)
		events := recorder.Events()

		failures[0].Datagram.Payload[0] = 'X'
		events[0] = "changed"

		assert.Equal(t, []byte("frame"), recorder.Failures()[0].Datagram.Payload)
		assert.Equal(t, []string{`handler failed: "frame": boom`}, recorder.Events())
	})
}
