package udp_test

import (
	"bytes"
	"context"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource"
	"github.com/grandper/go-athanor/resource/server/udp"
	"github.com/grandper/go-athanor/resource/server/udp/fixture"
)

// quietPeriod is how long a test waits to be confident that a hook does not fire.
const quietPeriod = 100 * time.Millisecond

func TestOnHandlerFailed(t *testing.T) {
	ctx := context.Background()

	t.Run("should receive the datagram the handler failed to process, with its source, and the error",
		func(t *testing.T) {
			errHandle := errors.New("unknown frame type 0x7f")
			recorder := fixture.NewErrorRecorder()
			server := udp.NewServer(newPlainConfig(), fixture.NewRecordingHandler().FailWith(errHandle)).
				WithHooks(recorder.Hooks()...)
			initServer(ctx, t, server)
			sender := fixture.ListenUDPOnFreePort(t)

			fixture.SendDatagramFrom(t, sender, server.LocalAddress(), []byte("frame"))

			assert.Equal(t, []string{`handler failed: "frame": unknown frame type 0x7f`},
				recorder.WaitForEvents(t, 1, receiveTimeout))
			failure := recorder.Failures()[0]
			assert.Equal(t, []byte("frame"), failure.Datagram.Payload)
			assert.Equal(t, netip.MustParseAddr("127.0.0.1"), failure.Datagram.Source.Addr().Unmap())
			assert.Equal(t, fixture.PortOf(t, sender.LocalAddr()), int(failure.Datagram.Source.Port()))
			assert.ErrorIs(t, failure.Err, errHandle)
		})

	t.Run("should keep serving the next datagram after the handler failed", func(t *testing.T) {
		handler := fixture.NewRecordingHandler().FailWith(errors.New("boom"))
		recorder := fixture.NewErrorRecorder()
		server := udp.NewServer(newPlainConfig(), handler).WithHooks(recorder.Hooks()...)
		initServer(ctx, t, server)

		fixture.SendDatagram(t, server.LocalAddress(), []byte("first"))
		recorder.WaitForFailures(t, 1, receiveTimeout)
		fixture.SendDatagram(t, server.LocalAddress(), []byte("second"))

		assert.Equal(t, []string{
			`handler failed: "first": boom`,
			`handler failed: "second": boom`,
		}, recorder.WaitForEvents(t, 2, receiveTimeout))
		assert.Equal(t, 2, handler.Calls())
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
	})

	t.Run("should hand the hook the payload slice the handler received", func(t *testing.T) {
		// The handler owns the payload and decodes it in place before it fails: the hook sees its changes.
		handler := fixture.NewRecordingHandler().WithHandleFunc(func(_ context.Context, datagram udp.Datagram) error {
			copy(datagram.Payload, bytes.ToUpper(datagram.Payload))
			return errors.New("unknown frame")
		})
		recorder := fixture.NewErrorRecorder()
		server := udp.NewServer(newPlainConfig(), handler).WithHooks(recorder.Hooks()...)
		initServer(ctx, t, server)

		fixture.SendDatagram(t, server.LocalAddress(), []byte("frame"))

		assert.Equal(t, []byte("FRAME"), recorder.WaitForFailures(t, 1, receiveTimeout)[0].Datagram.Payload)
	})

	t.Run("should not fire when the handler succeeds", func(t *testing.T) {
		handler := fixture.NewRecordingHandler()
		recorder := fixture.NewErrorRecorder()
		server := udp.NewServer(newPlainConfig(), handler).WithHooks(recorder.Hooks()...)
		initServer(ctx, t, server)

		fixture.SendDatagram(t, server.LocalAddress(), []byte("frame"))

		handler.WaitForDatagram(t, receiveTimeout)
		assert.Never(t, func() bool { return len(recorder.Events()) > 0 }, quietPeriod, statusTick)
	})

	t.Run("should run every hook, in registration order, across several WithHooks calls", func(t *testing.T) {
		recorder := fixture.NewErrorRecorder()
		server := udp.NewServer(newPlainConfig(), fixture.NewRecordingHandler().FailWith(errors.New("boom"))).
			WithHooks(recorder.HooksLabeled("first")...).
			WithHooks(append(recorder.HooksLabeled("second"), recorder.HooksLabeled("third")...)...)
		initServer(ctx, t, server)

		fixture.SendDatagram(t, server.LocalAddress(), []byte("frame"))

		assert.Equal(t, []string{
			`first: handler failed: "frame": boom`,
			`second: handler failed: "frame": boom`,
			`third: handler failed: "frame": boom`,
		}, recorder.WaitForEvents(t, 3, receiveTimeout))
	})

	t.Run("should ignore a nil callback and still run the other hooks", func(t *testing.T) {
		recorder := fixture.NewErrorRecorder()
		server := udp.NewServer(newPlainConfig(), fixture.NewRecordingHandler().FailWith(errors.New("boom"))).
			WithHooks(udp.OnHandlerFailed(nil)).
			WithHooks(recorder.Hooks()...).
			WithHooks(udp.OnHandlerFailed(nil))
		initServer(ctx, t, server)

		fixture.SendDatagram(t, server.LocalAddress(), []byte("first"))
		recorder.WaitForFailures(t, 1, receiveTimeout)
		fixture.SendDatagram(t, server.LocalAddress(), []byte("second"))

		assert.Equal(t, []string{
			`handler failed: "first": boom`,
			`handler failed: "second": boom`,
		}, recorder.WaitForEvents(t, 2, receiveTimeout), "a nil callback must neither panic nor stop the read loop")
	})

	t.Run("should let hooks be added while datagrams are handled", func(t *testing.T) {
		const datagramCount = 50
		handler := fixture.NewRecordingHandler().FailWith(errors.New("boom"))
		server := udp.NewServer(newPlainConfig(), handler).WithHooks(fixture.NewErrorRecorder().Hooks()...)
		initServer(ctx, t, server)

		// The race detector fails the test when the read loop reads the hooks without synchronization.
		var group sync.WaitGroup
		group.Add(2)
		go func() {
			defer group.Done()
			for range datagramCount {
				server.WithHooks(fixture.NewErrorRecorder().Hooks()...)
			}
		}()
		go func() {
			defer group.Done()
			for range datagramCount {
				fixture.SendDatagram(t, server.LocalAddress(), []byte("frame"))
			}
		}()
		group.Wait()

		require.Eventually(t, func() bool { return handler.Calls() > 0 }, receiveTimeout, statusTick)
	})
}
