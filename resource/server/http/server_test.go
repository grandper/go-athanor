package http_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	athanorfixture "github.com/grandper/go-athanor/fixture"
	"github.com/grandper/go-athanor/resource"
	httpserver "github.com/grandper/go-athanor/resource/server/http"
	"github.com/grandper/go-athanor/resource/server/http/fixture"
)

func TestServer(t *testing.T) {
	ctx := context.Background()

	t.Run("should report StatusInitializing before Init", func(t *testing.T) {
		server := httpserver.NewServer(&httpserver.Config{BindInterface: "127.0.0.1", BindPort: 8080})

		assert.Equal(t, resource.StatusInitializing, server.Status().Get())
	})

	t.Run("should use the default name", func(t *testing.T) {
		server := httpserver.NewServer(&httpserver.Config{BindInterface: "127.0.0.1", BindPort: 8080})

		assert.Equal(t, httpserver.DefaultName, server.Name())
	})

	t.Run("should use the name set with WithName", func(t *testing.T) {
		server := httpserver.NewServer(&httpserver.Config{BindInterface: "127.0.0.1", BindPort: 8080}).
			WithName("public-http")

		assert.Equal(t, "public-http", server.Name())
	})

	t.Run("should register the handlers and report StatusHealthy", func(t *testing.T) {
		handler := fixture.NewFakeHandler()
		config := &httpserver.Config{BindInterface: "127.0.0.1"}
		server := httpserver.NewServer(config, handler)

		err := fixture.InitOnFreePort(ctx, t, config, server.Init)

		require.NoError(t, err)
		defer func() { require.NoError(t, server.Shutdown(ctx)) }()
		assert.True(t, handler.WasRegistered())
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
	})

	t.Run("fails to init when a handler cannot register", func(t *testing.T) {
		errRegister := errors.New("registration failed")
		config := &httpserver.Config{BindInterface: "127.0.0.1", BindPort: 8080}
		server := httpserver.NewServer(config, fixture.NewFakeHandler().FailRegisterWith(errRegister))

		err := server.Init(ctx)

		require.ErrorIs(t, err, errRegister)
		require.ErrorContains(t, err, "failed to register an HTTP handler")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init when the bind interface is empty", func(t *testing.T) {
		server := httpserver.NewServer(&httpserver.Config{BindPort: 8080})

		err := server.Init(ctx)

		require.ErrorIs(t, err, httpserver.ErrEmptyBindInterface)
		require.ErrorContains(t, err, "failed to init the HTTP server")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init when the bind port is not initialized", func(t *testing.T) {
		server := httpserver.NewServer(&httpserver.Config{BindInterface: "127.0.0.1"})

		err := server.Init(ctx)

		require.ErrorIs(t, err, httpserver.ErrBindPortNotInitialized)
		require.ErrorContains(t, err, "failed to init the HTTP server")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init when the address is already in use", func(t *testing.T) {
		occupied := athanorfixture.ListenOnFreePort(t)
		config := &httpserver.Config{BindInterface: "127.0.0.1", BindPort: athanorfixture.PortOf(t, occupied)}
		server := httpserver.NewServer(config)

		err := server.Init(ctx)

		require.ErrorIs(t, err, syscall.EADDRINUSE)
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to init twice, leaving the running server untouched", func(t *testing.T) {
		config := &httpserver.Config{BindInterface: "127.0.0.1"}
		server := httpserver.NewServer(config)
		require.NoError(t, fixture.InitOnFreePort(ctx, t, config, server.Init))
		defer func() { require.NoError(t, server.Shutdown(ctx)) }()

		err := server.Init(ctx)

		require.ErrorIs(t, err, httpserver.ErrServerAlreadyStarted)
		require.ErrorContains(t, err, "failed to init the HTTP server")
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
	})

	t.Run("should be initializable again after a failed Init", func(t *testing.T) {
		config := &httpserver.Config{BindInterface: "127.0.0.1"}
		server := httpserver.NewServer(config)
		require.Error(t, server.Init(ctx))

		err := fixture.InitOnFreePort(ctx, t, config, server.Init)

		require.NoError(t, err)
		defer func() { require.NoError(t, server.Shutdown(ctx)) }()
		assert.Equal(t, resource.StatusHealthy, server.Status().Get())
	})

	t.Run("should report StatusClosed after a graceful shutdown", func(t *testing.T) {
		config := &httpserver.Config{BindInterface: "127.0.0.1"}
		server := httpserver.NewServer(config)
		require.NoError(t, fixture.InitOnFreePort(ctx, t, config, server.Init))

		err := server.Shutdown(ctx)

		require.NoError(t, err)
		assert.Equal(t, resource.StatusClosed, server.Status().Get())
	})

	t.Run("fails to init again after a shutdown, staying closed", func(t *testing.T) {
		config := &httpserver.Config{BindInterface: "127.0.0.1"}
		server := httpserver.NewServer(config)
		require.NoError(t, fixture.InitOnFreePort(ctx, t, config, server.Init))
		require.NoError(t, server.Shutdown(ctx))

		err := server.Init(ctx)

		require.ErrorIs(t, err, httpserver.ErrServerAlreadyStarted)
		assert.Equal(t, resource.StatusClosed, server.Status().Get())
	})

	t.Run("should report StatusClosed when Init was never called", func(t *testing.T) {
		server := httpserver.NewServer(&httpserver.Config{BindInterface: "127.0.0.1", BindPort: 8080})

		err := server.Shutdown(ctx)

		require.NoError(t, err)
		assert.Equal(t, resource.StatusClosed, server.Status().Get())
	})

	t.Run("should report StatusFailed when the serve loop fails", func(t *testing.T) {
		// Unreadable TLS files make the serve loop fail.
		config := &httpserver.Config{
			BindInterface: "127.0.0.1",
			TLSCertFile:   "testdata/missing-cert.pem",
			TLSKeyFile:    "testdata/missing-key.pem",
		}
		server := httpserver.NewServer(config)

		require.NoError(t, fixture.InitOnFreePort(ctx, t, config, server.Init))
		require.Eventually(t, func() bool { return server.Status().Get() == resource.StatusFailed },
			5*time.Second, 5*time.Millisecond, "the serve failure never reached the status")

		err := server.Shutdown(ctx)

		require.ErrorIs(t, err, fs.ErrNotExist)
		require.ErrorContains(t, err, "failed to serve HTTP")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("fails to shut down when the context expires before the in-flight requests complete", func(t *testing.T) {
		config := &httpserver.Config{BindInterface: "127.0.0.1"}
		started := make(chan struct{})
		release := make(chan struct{})
		defer close(release)
		slow := fixture.NewFakeHandler().WithRegisterFunc(func(router *mux.Router) error {
			router.HandleFunc("/slow", func(http.ResponseWriter, *http.Request) {
				close(started)
				<-release
			})
			return nil
		})
		server := httpserver.NewServer(config, slow)
		require.NoError(t, fixture.InitOnFreePort(ctx, t, config, server.Init))

		go func() {
			response, err := http.Get(fmt.Sprintf("http://%s/slow", config.Address()))
			if err == nil {
				_ = response.Body.Close()
			}
		}()
		athanorfixture.Receive(t, started, 5*time.Second, "the request never reached the handler")
		shutdownCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		err := server.Shutdown(shutdownCtx)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.ErrorContains(t, err, "failed to shut down the HTTP server")
		assert.Equal(t, resource.StatusFailed, server.Status().Get())
	})

	t.Run("should be manageable by a resource.List", func(t *testing.T) {
		config := &httpserver.Config{BindInterface: "127.0.0.1"}
		server := httpserver.NewServer(config)

		list := resource.NewList()
		require.NoError(t, list.Register(server))
		require.NoError(t, fixture.InitOnFreePort(ctx, t, config, list.Init))
		assert.Equal(t, resource.StatusHealthy, list.Status().Get())

		require.NoError(t, list.Shutdown(ctx))
		assert.Equal(t, resource.StatusClosed, list.Status().Get())
	})
}
