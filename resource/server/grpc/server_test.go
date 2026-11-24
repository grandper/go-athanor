package grpc_test

import (
	"context"
	"errors"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	athanorfixture "github.com/grandper/go-athanor/fixture"
	"github.com/grandper/go-athanor/resource"
	grpcserver "github.com/grandper/go-athanor/resource/server/grpc"
	"github.com/grandper/go-athanor/resource/server/grpc/fixture"
)

func TestServer(t *testing.T) {
	ctx := context.Background()

	t.Run("should report StatusInitializing before Init", func(t *testing.T) {
		server := grpcserver.NewServer(&grpcserver.Config{BindInterface: "127.0.0.1", BindPort: 50051})

		assert.Equal(t, resource.StatusInitializing, server.Status().Get())
	})

	t.Run("should use the default name", func(t *testing.T) {
		server := grpcserver.NewServer(&grpcserver.Config{BindInterface: "127.0.0.1", BindPort: 50051})

		assert.Equal(t, grpcserver.DefaultName, server.Name())
	})

	t.Run("should use the name set with WithName", func(t *testing.T) {
		config := &grpcserver.Config{BindInterface: "127.0.0.1", BindPort: 50051}
		server := grpcserver.NewServer(config).WithName("public-grpc")

		assert.Equal(t, "public-grpc", server.Name())
	})

	t.Run("should return the same server from WithHooks", func(t *testing.T) {
		server := newHealthServer()

		assert.Same(t, server, server.WithHooks(fixture.NewHookRecorder().Hooks()...))
	})

	t.Run("should keep the hooks of every WithHooks call", func(t *testing.T) {
		first, second := fixture.NewHookRecorder(), fixture.NewHookRecorder()
		server := newHealthServer().WithHooks(first.Hooks()...)
		server.WithHooks(second.Hooks()...)
		client := serveOnFreePort(t, server)

		_, err := checkHealth(t, client, "orders")

		require.Error(t, err)
		want := []string{"handler failed: /grpc.health.v1.Health/Check: NotFound"}
		assert.Equal(t, want, first.EventsContaining("handler "))
		assert.Equal(t, want, second.EventsContaining("handler "))
	})

	t.Run("should ignore the hooks added after Init", func(t *testing.T) {
		early, late := fixture.NewHookRecorder(), fixture.NewHookRecorder()
		server := newHealthServer().WithHooks(early.Hooks()...)
		client := serveOnFreePort(t, server)

		server.WithHooks(late.Hooks()...)
		_, err := checkHealth(t, client, "orders")

		require.Error(t, err)
		assert.Equal(t, []string{"handler failed: /grpc.health.v1.Health/Check: NotFound"},
			early.EventsContaining("handler "))
		assert.Empty(t, late.Events(), "a hook added after Init must be ignored")
	})

	t.Run("should let hooks be added while requests are served", func(t *testing.T) {
		const requestCount = 20
		server := newHealthServer().WithHooks(fixture.NewHookRecorder().Hooks()...)
		client := serveOnFreePort(t, server)

		// The race detector fails the test when the requests read the hooks without synchronization.
		var group sync.WaitGroup
		group.Add(2)
		go func() {
			defer group.Done()
			for range requestCount {
				server.WithHooks(fixture.NewHookRecorder().Hooks()...)
			}
		}()
		go func() {
			defer group.Done()
			for range requestCount {
				_, err := checkHealth(t, client, "orders")
				assert.Equal(t, codes.NotFound, status.Code(err))
			}
		}()
		group.Wait()
	})

	t.Run("should register the handlers and report StatusHealthy", func(t *testing.T) {
		handler := fixture.NewFakeHandler()
		server := grpcserver.NewServer(&grpcserver.Config{}, handler).
			WithListener(athanorfixture.ListenOnFreePort(t))

		err := server.Init(ctx)

		require.NoError(t, err)
		defer func() { require.NoError(t, server.Shutdown(ctx)) }()
		assert.True(t, handler.WasRegistered())
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
	})

	t.Run("fails to init when a handler cannot register", func(t *testing.T) {
		errRegister := errors.New("registration failed")
		listener := athanorfixture.ListenOnFreePort(t)
		server := grpcserver.NewServer(&grpcserver.Config{}, fixture.NewFakeHandler().FailRegisterWith(errRegister)).
			WithListener(listener)

		err := server.Init(ctx)

		require.ErrorIs(t, err, errRegister)
		require.ErrorContains(t, err, "failed to register a gRPC handler")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init when the bind interface is empty", func(t *testing.T) {
		server := grpcserver.NewServer(&grpcserver.Config{BindPort: 50051})

		err := server.Init(ctx)

		require.ErrorIs(t, err, grpcserver.ErrEmptyBindInterface)
		require.ErrorContains(t, err, "failed to init the gRPC server")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init when the bind port is not initialized", func(t *testing.T) {
		server := grpcserver.NewServer(&grpcserver.Config{BindInterface: "127.0.0.1"})

		err := server.Init(ctx)

		require.ErrorIs(t, err, grpcserver.ErrBindPortNotInitialized)
		require.ErrorContains(t, err, "failed to init the gRPC server")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init when the address is already in use", func(t *testing.T) {
		occupied := athanorfixture.ListenOnFreePort(t)
		config := &grpcserver.Config{BindInterface: "127.0.0.1", BindPort: athanorfixture.PortOf(t, occupied)}
		server := grpcserver.NewServer(config)

		err := server.Init(ctx)

		require.ErrorIs(t, err, syscall.EADDRINUSE)
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("should serve on the listener set with WithListener instead of the listen address", func(t *testing.T) {
		listener := athanorfixture.ListenOnFreePort(t)
		server := grpcserver.NewServer(&grpcserver.Config{}, grpcserver.NewHealthHandler(resource.NewList())).
			WithListener(listener)

		err := server.Init(ctx)

		require.NoError(t, err)
		defer func() { require.NoError(t, server.Shutdown(ctx)) }()
		fixture.AssertEventuallyServing(t, fixture.NewHealthClient(t, listener.Addr().String()), "")
	})

	t.Run("should report StatusFailed when serving fails", func(t *testing.T) {
		listener := athanorfixture.NewFailingListener(errors.New("accept failed"))
		server := grpcserver.NewServer(&grpcserver.Config{}).WithListener(listener)

		require.NoError(t, server.Init(ctx))
		defer func() { _ = server.Shutdown(ctx) }()

		assert.Eventually(t, func() bool { return server.Status().Get() == resource.StatusFailed },
			5*time.Second, 5*time.Millisecond, "the serve failure never reached the status")
	})

	t.Run("fails to shut down when serving failed", func(t *testing.T) {
		errAccept := errors.New("accept failed")
		listener := athanorfixture.NewFailingListener(errAccept)
		server := grpcserver.NewServer(&grpcserver.Config{}).WithListener(listener)
		require.NoError(t, server.Init(ctx))
		// A shutdown that wins the race against the serve goroutine is a clean stop: wait for the failure.
		require.Eventually(t, func() bool { return server.Status().Get() == resource.StatusFailed },
			5*time.Second, 5*time.Millisecond, "the serve failure never reached the status")

		err := server.Shutdown(ctx)

		require.ErrorIs(t, err, errAccept)
		require.ErrorContains(t, err, "failed to serve gRPC")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
		assert.True(t, listener.WasClosed())
	})

	t.Run("fails to init twice, leaving the running server untouched", func(t *testing.T) {
		server := grpcserver.NewServer(&grpcserver.Config{}).WithListener(athanorfixture.ListenOnFreePort(t))
		require.NoError(t, server.Init(ctx))
		defer func() { require.NoError(t, server.Shutdown(ctx)) }()

		err := server.Init(ctx)

		require.ErrorIs(t, err, grpcserver.ErrServerAlreadyStarted)
		require.ErrorContains(t, err, "failed to init the gRPC server")
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
	})

	t.Run("should be initializable again after a failed Init", func(t *testing.T) {
		config := &grpcserver.Config{}
		server := grpcserver.NewServer(config)
		require.Error(t, server.Init(ctx))

		config.BindInterface = "127.0.0.1"
		err := athanorfixture.OnFreePort(t, func(port int) error {
			config.BindPort = port
			return server.Init(ctx)
		})

		require.NoError(t, err)
		defer func() { require.NoError(t, server.Shutdown(ctx)) }()
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
	})

	t.Run("should report StatusClosed after a graceful shutdown", func(t *testing.T) {
		server := grpcserver.NewServer(&grpcserver.Config{}).WithListener(athanorfixture.ListenOnFreePort(t))
		require.NoError(t, server.Init(ctx))

		err := server.Shutdown(ctx)

		require.NoError(t, err)
		assert.Equal(t, resource.StatusClosed, server.Status().Get())
	})

	t.Run("fails to init again after a shutdown, staying closed", func(t *testing.T) {
		server := grpcserver.NewServer(&grpcserver.Config{}).WithListener(athanorfixture.ListenOnFreePort(t))
		require.NoError(t, server.Init(ctx))
		require.NoError(t, server.Shutdown(ctx))

		err := server.Init(ctx)

		require.ErrorIs(t, err, grpcserver.ErrServerAlreadyStarted)
		assert.Equal(t, resource.StatusClosed, server.Status().Get())
	})

	t.Run("should stop immediately when the context expires before the in-flight calls complete", func(t *testing.T) {
		listener := athanorfixture.ListenOnFreePort(t)
		server := grpcserver.NewServer(&grpcserver.Config{}, grpcserver.NewHealthHandler(resource.NewList())).
			WithListener(listener)
		require.NoError(t, server.Init(ctx))

		// An open Watch stream keeps a graceful stop waiting forever.
		streamCtx, cancelStream := context.WithCancel(ctx)
		defer cancelStream()
		stream, err := fixture.NewHealthClient(t, listener.Addr().String()).Watch(streamCtx,
			&grpc_health_v1.HealthCheckRequest{})
		require.NoError(t, err)
		_, err = stream.Recv()
		require.NoError(t, err)

		shutdownCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		err = server.Shutdown(shutdownCtx)

		require.NoError(t, err)
		assert.Equal(t, resource.StatusClosed, server.Status().Get())
		_, err = stream.Recv()
		assert.NotEqual(t, codes.OK, status.Code(err), "the forced stop must cut the open stream")
	})

	t.Run("should report StatusClosed when Init was never called", func(t *testing.T) {
		server := grpcserver.NewServer(&grpcserver.Config{BindInterface: "127.0.0.1", BindPort: 50051})

		err := server.Shutdown(ctx)

		require.NoError(t, err)
		assert.Equal(t, resource.StatusClosed, server.Status().Get())
	})

	t.Run("should be manageable by a resource.List", func(t *testing.T) {
		server := grpcserver.NewServer(&grpcserver.Config{}).WithListener(athanorfixture.ListenOnFreePort(t))

		list := resource.NewList()
		require.NoError(t, list.Register(server))
		require.NoError(t, list.Init(ctx))
		assert.Equal(t, resource.StatusHealthy, list.Status().Get())

		require.NoError(t, list.Shutdown(ctx))
		assert.Equal(t, resource.StatusClosed, list.Status().Get())
	})
}
