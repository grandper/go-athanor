package udp_test

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	athanorfixture "github.com/grandper/go-athanor/fixture"
	"github.com/grandper/go-athanor/resource"
	"github.com/grandper/go-athanor/resource/server/udp"
	"github.com/grandper/go-athanor/resource/server/udp/fixture"
)

const (
	receiveTimeout = 5 * time.Second
	statusTick     = 5 * time.Millisecond
	bufferSize     = 1500
)

// ctxKey is the key of the value the tests carry through the contexts.
type ctxKey struct{}

// newPlainConfig returns the configuration of a plain UDP server bound to a free loopback port, without watchdog.
func newPlainConfig() *udp.Config {
	return &udp.Config{Address: "127.0.0.1:0", BufferSize: bufferSize}
}

// initServer initializes the server and shuts it down when the test ends.
func initServer(ctx context.Context, t *testing.T, server *udp.Server) {
	t.Helper()

	require.NoError(t, server.Init(ctx))
	t.Cleanup(func() { _ = server.Shutdown(ctx) })
}

// waitForStatus waits until the server reports the status.
func waitForStatus(t *testing.T, server *udp.Server, status resource.Status) {
	t.Helper()

	require.Eventually(t, func() bool { return server.Status().Get() == status }, receiveTimeout, statusTick,
		"the server never reported %s, last status: %s", status, server.Status().Get())
}

func TestServer(t *testing.T) {
	ctx := context.Background()

	t.Run("should report StatusInitializing before Init", func(t *testing.T) {
		server := udp.NewServer(newPlainConfig(), fixture.NewRecordingHandler())

		assert.Equal(t, resource.StatusInitializing, server.Status().Get())
		assert.Nil(t, server.LocalAddress())
	})

	t.Run("should use the default name", func(t *testing.T) {
		server := udp.NewServer(newPlainConfig(), fixture.NewRecordingHandler())

		assert.Equal(t, udp.DefaultName, server.Name())
	})

	t.Run("should use the name set with WithName", func(t *testing.T) {
		server := udp.NewServer(newPlainConfig(), fixture.NewRecordingHandler()).WithName("mv-primary")

		assert.Equal(t, "mv-primary", server.Name())
	})

	t.Run("should bind the address and report StatusHealthy", func(t *testing.T) {
		server := udp.NewServer(newPlainConfig(), fixture.NewRecordingHandler())

		err := server.Init(ctx)

		require.NoError(t, err)
		defer func() { require.NoError(t, server.Shutdown(ctx)) }()
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
		assert.Positive(t, fixture.PortOf(t, server.LocalAddress()))
	})

	t.Run("should hand the datagrams over to the handler", func(t *testing.T) {
		handler := fixture.NewRecordingHandler()
		server := udp.NewServer(newPlainConfig(), handler)
		initServer(ctx, t, server)

		fixture.SendDatagram(t, server.LocalAddress(), []byte("hello"))

		datagram := handler.WaitForDatagram(t, receiveTimeout)
		assert.Equal(t, []byte("hello"), datagram.Payload)
		assert.Equal(t, netip.MustParseAddr("127.0.0.1"), datagram.Source.Addr().Unmap())
		assert.Positive(t, datagram.Source.Port())
	})

	t.Run("should truncate the datagrams to the buffer size", func(t *testing.T) {
		handler := fixture.NewRecordingHandler()
		config := newPlainConfig()
		config.BufferSize = 4
		server := udp.NewServer(config, handler)
		initServer(ctx, t, server)

		fixture.SendDatagram(t, server.LocalAddress(), []byte("hello"))

		assert.Equal(t, []byte("hell"), handler.WaitForDatagram(t, receiveTimeout).Payload)
	})

	t.Run("should report the handler errors to the OnError callback and keep serving", func(t *testing.T) {
		errHandle := errors.New("handling failed")
		handler := fixture.NewRecordingHandler().FailWith(errHandle)
		server := udp.NewServer(newPlainConfig(), handler)
		reported := make(chan error, 1)
		server.OnError(func(err error) { reported <- err })
		initServer(ctx, t, server)

		fixture.SendDatagram(t, server.LocalAddress(), []byte("first"))
		assert.Equal(t, []byte("first"), handler.WaitForDatagram(t, receiveTimeout).Payload)

		err := athanorfixture.Receive(t, reported, receiveTimeout, "the error was never reported")

		require.ErrorIs(t, err, errHandle)
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
		fixture.SendDatagram(t, server.LocalAddress(), []byte("second"))
		assert.Equal(t, []byte("second"), handler.WaitForDatagram(t, receiveTimeout).Payload)
	})

	t.Run("should keep serving when a handler fails and no callback is set", func(t *testing.T) {
		handler := fixture.NewRecordingHandler().FailWith(assert.AnError)
		server := udp.NewServer(newPlainConfig(), handler)
		initServer(ctx, t, server)

		fixture.SendDatagram(t, server.LocalAddress(), []byte("first"))
		assert.Equal(t, []byte("first"), handler.WaitForDatagram(t, receiveTimeout).Payload)
		fixture.SendDatagram(t, server.LocalAddress(), []byte("second"))

		assert.Equal(t, []byte("second"), handler.WaitForDatagram(t, receiveTimeout).Payload)
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
	})

	t.Run("should hand the handler a context carrying the values of the Init context", func(t *testing.T) {
		contexts := make(chan context.Context, 1)
		handler := fixture.NewRecordingHandler().WithHandleFunc(func(ctx context.Context, _ udp.Datagram) error {
			contexts <- ctx
			return nil
		})
		server := udp.NewServer(newPlainConfig(), handler)
		initCtx, cancelInit := context.WithCancel(context.WithValue(ctx, ctxKey{}, "value"))
		defer cancelInit()
		initServer(initCtx, t, server)

		fixture.SendDatagram(t, server.LocalAddress(), []byte("hello"))

		handlerCtx := athanorfixture.Receive(t, contexts, receiveTimeout, "the handler was never called")
		assert.Equal(t, "value", handlerCtx.Value(ctxKey{}))
		cancelInit()
		require.NoError(t, handlerCtx.Err(), "the cancellation of the Init context reached the handler")
		require.NoError(t, server.Shutdown(ctx))
		assert.ErrorIs(t, handlerCtx.Err(), context.Canceled, "the shutdown did not cancel the handler context")
	})

	t.Run("fails to init when the config is nil", func(t *testing.T) {
		server := udp.NewServer(nil, fixture.NewRecordingHandler())

		err := server.Init(ctx)

		require.ErrorIs(t, err, udp.ErrNilConfig)
		require.ErrorContains(t, err, "failed to init the UDP server")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init when the handler is nil", func(t *testing.T) {
		server := udp.NewServer(newPlainConfig(), nil)

		err := server.Init(ctx)

		require.ErrorIs(t, err, udp.ErrNilHandler)
		require.ErrorContains(t, err, "failed to init the UDP server")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init when the address is empty", func(t *testing.T) {
		server := udp.NewServer(&udp.Config{BufferSize: bufferSize}, fixture.NewRecordingHandler())

		err := server.Init(ctx)

		require.ErrorIs(t, err, udp.ErrEmptyAddress)
		require.ErrorContains(t, err, "failed to init the UDP server")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init when the interface name is empty while multicast is enabled", func(t *testing.T) {
		config := &udp.Config{
			Address:    net.JoinHostPort(fixture.MulticastGroup, "0"),
			Multicast:  true,
			BufferSize: bufferSize,
		}
		server := udp.NewServer(config, fixture.NewRecordingHandler())

		err := server.Init(ctx)

		require.ErrorIs(t, err, udp.ErrEmptyInterfaceName)
		require.ErrorContains(t, err, "failed to init the UDP server")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init when the buffer size is not positive", func(t *testing.T) {
		for _, size := range []int{0, -1} {
			config := &udp.Config{Address: "127.0.0.1:0", BufferSize: size}
			server := udp.NewServer(config, fixture.NewRecordingHandler())

			err := server.Init(ctx)

			require.ErrorIs(t, err, udp.ErrInvalidBufferSize, size)
			require.ErrorContains(t, err, "failed to init the UDP server")
			assert.Equal(t, resource.StatusFailed, server.Status().Get(), size)
		}
	})

	t.Run("fails to init when the received timeout is negative", func(t *testing.T) {
		config := newPlainConfig()
		config.ReceivedTimeout = -time.Second
		server := udp.NewServer(config, fixture.NewRecordingHandler())

		err := server.Init(ctx)

		require.ErrorIs(t, err, udp.ErrInvalidReceivedTimeout)
		require.ErrorContains(t, err, "failed to init the UDP server")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init when the address cannot be resolved", func(t *testing.T) {
		config := &udp.Config{Address: "not-an-address", BufferSize: bufferSize}
		server := udp.NewServer(config, fixture.NewRecordingHandler())

		err := server.Init(ctx)

		require.ErrorContains(t, err, `failed to resolve "not-an-address"`)
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init when the address is already in use", func(t *testing.T) {
		occupied := fixture.ListenUDPOnFreePort(t)
		config := &udp.Config{Address: occupied.LocalAddr().String(), BufferSize: bufferSize}
		server := udp.NewServer(config, fixture.NewRecordingHandler())

		err := server.Init(ctx)

		require.ErrorIs(t, err, syscall.EADDRINUSE)
		require.ErrorContains(t, err, "failed to listen on")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init when the multicast interface is unknown", func(t *testing.T) {
		config := &udp.Config{
			Address:       net.JoinHostPort(fixture.MulticastGroup, "0"),
			Multicast:     true,
			InterfaceName: "no-such-interface",
			BufferSize:    bufferSize,
		}
		server := udp.NewServer(config, fixture.NewRecordingHandler())

		err := server.Init(ctx)

		require.ErrorContains(t, err, `failed to find the interface "no-such-interface"`)
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init twice, leaving the running server untouched", func(t *testing.T) {
		server := udp.NewServer(newPlainConfig(), fixture.NewRecordingHandler())
		initServer(ctx, t, server)
		address := server.LocalAddress()

		err := server.Init(ctx)

		require.ErrorIs(t, err, udp.ErrServerAlreadyStarted)
		require.ErrorContains(t, err, "failed to init the UDP server")
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
		assert.Equal(t, address, server.LocalAddress())
	})

	t.Run("should be initializable again after a failed Init", func(t *testing.T) {
		occupied := fixture.ListenUDPOnFreePort(t)
		config := &udp.Config{Address: occupied.LocalAddr().String(), BufferSize: bufferSize}
		server := udp.NewServer(config, fixture.NewRecordingHandler())
		require.Error(t, server.Init(ctx))
		require.NoError(t, occupied.Close())

		err := server.Init(ctx)

		require.NoError(t, err)
		defer func() { require.NoError(t, server.Shutdown(ctx)) }()
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
	})

	t.Run("should report StatusClosed after a graceful shutdown", func(t *testing.T) {
		server := udp.NewServer(newPlainConfig(), fixture.NewRecordingHandler())
		require.NoError(t, server.Init(ctx))

		err := server.Shutdown(ctx)

		require.NoError(t, err)
		assert.Equal(t, resource.StatusClosed, server.Status().Get())
	})

	t.Run("should shut down only once, the second call changing nothing", func(t *testing.T) {
		server := udp.NewServer(newPlainConfig(), fixture.NewRecordingHandler())
		require.NoError(t, server.Init(ctx))
		require.NoError(t, server.Shutdown(ctx))

		err := server.Shutdown(ctx)

		require.NoError(t, err)
		assert.Equal(t, resource.StatusClosed, server.Status().Get())
	})

	t.Run("fails to init again after a shutdown, staying closed", func(t *testing.T) {
		server := udp.NewServer(newPlainConfig(), fixture.NewRecordingHandler())
		require.NoError(t, server.Init(ctx))
		require.NoError(t, server.Shutdown(ctx))

		err := server.Init(ctx)

		require.ErrorIs(t, err, udp.ErrServerAlreadyStarted)
		assert.Equal(t, resource.StatusClosed, server.Status().Get())
	})

	t.Run("should report StatusClosed when Init was never called", func(t *testing.T) {
		server := udp.NewServer(newPlainConfig(), fixture.NewRecordingHandler())

		err := server.Shutdown(ctx)

		require.NoError(t, err)
		assert.Equal(t, resource.StatusClosed, server.Status().Get())
	})

	t.Run("should report StatusFailed when reading fails, and fail to shut down", func(t *testing.T) {
		errRead := errors.New("read failed")
		conn := fixture.NewFailingConnection(errRead)
		config := &udp.Config{BufferSize: bufferSize}
		server := udp.NewServer(config, fixture.NewRecordingHandler()).WithConnection(conn)
		initServer(ctx, t, server)
		waitForStatus(t, server, resource.StatusFailed)

		err := server.Shutdown(ctx)

		require.ErrorIs(t, err, errRead)
		require.ErrorContains(t, err, "failed to serve UDP")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
		assert.True(t, conn.WasClosed())
	})

	t.Run("should report StatusFailed when no datagram arrives within the received timeout", func(t *testing.T) {
		config := newPlainConfig()
		config.ReceivedTimeout = 100 * time.Millisecond
		server := udp.NewServer(config, fixture.NewRecordingHandler())
		initServer(ctx, t, server)
		waitForStatus(t, server, resource.StatusFailed)

		err := server.Shutdown(ctx)

		require.ErrorIs(t, err, udp.ErrReceivedTimeout)
		require.ErrorContains(t, err, "no data received since 100ms")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("should stay healthy while datagrams keep arriving", func(t *testing.T) {
		config := newPlainConfig()
		config.ReceivedTimeout = 300 * time.Millisecond
		server := udp.NewServer(config, fixture.NewRecordingHandler())
		initServer(ctx, t, server)
		var statuses []resource.Status
		defer server.Status().Observe(resource.StatusObserverFunc(func(s resource.Status) {
			statuses = append(statuses, s)
		}))()

		for i := 0; i < 20; i++ {
			fixture.SendDatagram(t, server.LocalAddress(), []byte("tick"))
			time.Sleep(30 * time.Millisecond)
		}

		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
		assert.Empty(t, statuses, "the status changed meanwhile")
	})

	t.Run("should not watch the silence when the received timeout is zero", func(t *testing.T) {
		server := udp.NewServer(newPlainConfig(), fixture.NewRecordingHandler())
		initServer(ctx, t, server)

		assert.Never(t, func() bool { return server.Status().Get() != resource.StatusHealthy },
			200*time.Millisecond, statusTick)
	})

	t.Run("should make resource.Run fail with ErrReceivedTimeout when the silence lasts", func(t *testing.T) {
		config := newPlainConfig()
		config.ReceivedTimeout = 100 * time.Millisecond
		server := udp.NewServer(config, fixture.NewRecordingHandler())

		err := resource.Run(ctx, server)

		require.ErrorIs(t, err, udp.ErrReceivedTimeout)
		require.ErrorIs(t, err, resource.ErrResourceFailed)
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to shut down when the context expires before the handler returns", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		defer close(release)
		handler := fixture.NewRecordingHandler().WithHandleFunc(func(context.Context, udp.Datagram) error {
			close(started)
			<-release
			return nil
		})
		server := udp.NewServer(newPlainConfig(), handler)
		initServer(ctx, t, server)
		fixture.SendDatagram(t, server.LocalAddress(), []byte("slow"))
		athanorfixture.Receive(t, started, receiveTimeout, "the datagram never reached the handler")
		shutdownCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		err := server.Shutdown(shutdownCtx)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.ErrorContains(t, err, "failed to shut down the UDP server")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("should join the multicast group on the interface and receive its datagrams", func(t *testing.T) {
		iface := fixture.MulticastInterface(t)
		handler := fixture.NewRecordingHandler()
		config := &udp.Config{
			Address:       net.JoinHostPort(fixture.MulticastGroup, "0"),
			Multicast:     true,
			InterfaceName: iface.Name,
			BufferSize:    bufferSize,
		}
		server := udp.NewServer(config, handler)
		initServer(ctx, t, server)
		require.Equal(t, resource.StatusHealthy, server.Status().Get())
		group := &net.UDPAddr{IP: net.ParseIP(fixture.MulticastGroup), Port: fixture.PortOf(t, server.LocalAddress())}

		fixture.SendMulticastDatagram(t, iface, group, []byte("hello"))

		assert.Equal(t, []byte("hello"), handler.WaitForDatagram(t, receiveTimeout).Payload)
	})

	t.Run("should serve on the connection set with WithConnection, without an address", func(t *testing.T) {
		conn := fixture.ListenUDPOnFreePort(t)
		handler := fixture.NewRecordingHandler()
		config := &udp.Config{BufferSize: bufferSize}
		server := udp.NewServer(config, handler).WithConnection(conn)
		initServer(ctx, t, server)
		require.Equal(t, conn.LocalAddr(), server.LocalAddress())

		fixture.SendDatagram(t, conn.LocalAddr(), []byte("hello"))

		assert.Equal(t, []byte("hello"), handler.WaitForDatagram(t, receiveTimeout).Payload)
		require.NoError(t, server.Shutdown(ctx))
		assert.Equal(t, resource.StatusClosed, server.Status().Get())
	})

	t.Run("should be manageable by a resource.List", func(t *testing.T) {
		server := udp.NewServer(newPlainConfig(), fixture.NewRecordingHandler())

		list := resource.NewList()
		require.NoError(t, list.Register(server))
		require.NoError(t, list.Init(ctx))
		assert.Equal(t, resource.StatusHealthy, list.Status().Get())

		require.NoError(t, list.Shutdown(ctx))
		assert.Equal(t, resource.StatusClosed, list.Status().Get())
	})
}
