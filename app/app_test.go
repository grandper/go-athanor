package app_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/app"
	appfixture "github.com/grandper/go-athanor/app/fixture"
	athanorfixture "github.com/grandper/go-athanor/fixture"
	"github.com/grandper/go-athanor/process"
	"github.com/grandper/go-athanor/resource"
	resourcefixture "github.com/grandper/go-athanor/resource/fixture"
	grpcserver "github.com/grandper/go-athanor/resource/server/grpc"
	grpcfixture "github.com/grandper/go-athanor/resource/server/grpc/fixture"
	httpserver "github.com/grandper/go-athanor/resource/server/http"
	httpfixture "github.com/grandper/go-athanor/resource/server/http/fixture"
	natsserver "github.com/grandper/go-athanor/resource/server/nats"
	natsfixture "github.com/grandper/go-athanor/resource/server/nats/fixture"
)

func TestRun(t *testing.T) {
	ctx := context.Background()

	t.Run("should return immediately when nothing is registered", func(t *testing.T) {
		err := app.Run(ctx)

		require.NoError(t, err)
	})

	t.Run("should run the before hooks, the processes, and the after hooks in order", func(t *testing.T) {
		recorder := appfixture.NewStepRecorder()

		err := app.Run(ctx,
			app.WithHooksBefore(recorder.Step("before-1"), recorder.Step("before-2")),
			app.WithProcess(recorder.Step("process")),
			app.WithHookAfter(recorder.Step("after")),
		)

		require.NoError(t, err)
		assert.Equal(t, []string{"before-1", "before-2", "process", "after"}, recorder.Steps())
	})

	t.Run("fails without running processes or after hooks when a before hook fails", func(t *testing.T) {
		errHook := errors.New("boom")
		var processRan, afterRan bool

		err := app.Run(ctx,
			app.WithHookBefore(func(context.Context) error { return errHook }),
			app.WithProcess(func(context.Context) error { processRan = true; return nil }),
			app.WithHookAfter(func(context.Context) error { afterRan = true; return nil }),
		)

		require.ErrorIs(t, err, errHook)
		assert.False(t, processRan)
		assert.False(t, afterRan)
	})

	t.Run("should run the after hooks even when a process fails", func(t *testing.T) {
		errProcess := errors.New("process blew up")
		var afterRan bool

		err := app.Run(ctx,
			app.WithProcess(func(context.Context) error { return errProcess }),
			app.WithHookAfter(func(context.Context) error { afterRan = true; return nil }),
		)

		require.ErrorIs(t, err, errProcess)
		assert.True(t, afterRan)
	})

	t.Run("fails when an after hook fails", func(t *testing.T) {
		errHook := errors.New("cleanup failed")

		err := app.Run(ctx,
			app.WithHookAfter(func(context.Context) error { return errHook }),
		)

		require.ErrorIs(t, err, errHook)
	})

	t.Run("should run every after hook and report every failure when several fail", func(t *testing.T) {
		errFirst := errors.New("first cleanup failed")
		errLast := errors.New("last cleanup failed")
		recorder := appfixture.NewStepRecorder()

		err := app.Run(ctx,
			app.WithHooksAfter(
				recorder.FailingStep("after-1", errFirst),
				recorder.Step("after-2"),
				recorder.FailingStep("after-3", errLast),
			),
		)

		require.ErrorIs(t, err, errFirst)
		require.ErrorIs(t, err, errLast)
		require.ErrorContains(t, err, "after hook failed")
		assert.Equal(t, []string{"after-1", "after-2", "after-3"}, recorder.Steps())
	})

	t.Run("should give the after hooks a live, bounded context when cancellation stopped the app", func(t *testing.T) {
		type contextKey struct{}
		cancelCtx, cancel := context.WithCancel(context.WithValue(ctx, contextKey{}, "trace-42"))
		cancel()
		var hookErr error
		var hookValue any
		var hookHasDeadline bool

		err := app.Run(cancelCtx,
			app.WithHookAfter(func(hookCtx context.Context) error {
				hookErr = hookCtx.Err()
				hookValue = hookCtx.Value(contextKey{})
				_, hookHasDeadline = hookCtx.Deadline()
				return nil
			}),
		)

		require.NoError(t, err)
		require.NoError(t, hookErr, "a cleanup hook must not start with a canceled context")
		assert.Equal(t, "trace-42", hookValue)
		assert.True(t, hookHasDeadline, "the cleanup must be bounded")
	})

	t.Run("should cancel the remaining processes when one fails", func(t *testing.T) {
		errProcess := errors.New("process blew up")
		var canceled bool

		err := app.Run(ctx,
			app.WithProcesses(
				func(context.Context) error { return errProcess },
				func(ctx context.Context) error {
					<-ctx.Done()
					canceled = true
					return nil
				},
			),
		)

		require.ErrorIs(t, err, errProcess)
		assert.True(t, canceled)
	})

	t.Run("should treat a POSIX signal as a graceful shutdown", func(t *testing.T) {
		// Simulates the signal listener without raising a real signal.
		err := app.Run(ctx,
			app.WithProcess(func(context.Context) error { return process.ErrPOSIXSignalReceived }),
		)

		require.NoError(t, err, "a POSIX signal is a graceful shutdown, not an error")
	})

	t.Run("fails with the shutdown error when a resource cannot shut down after a POSIX signal", func(t *testing.T) {
		errShutdown := errors.New("connection pool is stuck")
		managed := resourcefixture.NewHealthyFakeManaged("database").FailShutdownWith(errShutdown)

		err := app.Run(ctx,
			app.WithResource(managed),
			app.WithProcess(func(context.Context) error { return process.ErrPOSIXSignalReceived }),
		)

		require.ErrorIs(t, err, errShutdown)
		assert.NotErrorIs(t, err, process.ErrPOSIXSignalReceived, "the signal itself is not an error")
	})

	t.Run("should stop the POSIX signal listener when the context is canceled", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)

		result := make(chan error, 1)
		go func() {
			result <- app.Run(cancelCtx, app.WithProcess(process.ListenToSignals))
		}()

		cancel()

		err := athanorfixture.Receive(t, result, 5*time.Second, "Run did not return after the context was canceled")
		require.NoError(t, err)
	})

	t.Run("should initialize the resources before any process starts", func(t *testing.T) {
		managed := resourcefixture.NewHealthyFakeManaged("database")
		var statusSeenByProcess resource.Status

		err := app.Run(ctx,
			app.WithResource(managed),
			app.WithProcess(func(context.Context) error {
				statusSeenByProcess = managed.Status().Get()
				return process.ErrPOSIXSignalReceived // stop the app once the check is done
			}),
		)

		require.NoError(t, err)
		assert.Equal(t, resource.StatusHealthy, statusSeenByProcess,
			"a process must be able to use the resources from its first instruction")
	})

	t.Run("should shut the resources down only after the processes have stopped", func(t *testing.T) {
		managed := resourcefixture.NewHealthyFakeManaged("database")
		var shutdownSeenByProcess bool

		err := app.Run(ctx,
			app.WithResource(managed),
			app.WithProcess(func(context.Context) error {
				shutdownSeenByProcess = managed.WasShutdown()
				return process.ErrPOSIXSignalReceived // stop the app once the check is done
			}),
		)

		require.NoError(t, err)
		assert.False(t, shutdownSeenByProcess, "no resource may be torn down under a running process")
		assert.True(t, managed.WasShutdown())
	})

	t.Run("should run the after hooks even when a resource fails to initialize", func(t *testing.T) {
		errInit := errors.New("the database is unreachable")
		var afterRan bool

		err := app.Run(ctx,
			app.WithResource(resourcefixture.NewFakeManaged("database").FailInitWith(errInit)),
			app.WithHookAfter(func(context.Context) error { afterRan = true; return nil }),
		)

		require.ErrorIs(t, err, errInit)
		assert.True(t, afterRan)
	})

	t.Run("should stop the processes when a resource fails", func(t *testing.T) {
		managed := resourcefixture.NewHealthyFakeManaged("database")
		var canceled bool

		err := app.Run(ctx,
			app.WithResource(managed),
			app.WithProcesses(
				// Simulates a resource failure.
				func(context.Context) error {
					managed.Notify(resource.StatusFailed)
					return nil
				},
				func(ctx context.Context) error {
					<-ctx.Done()
					canceled = true
					return nil
				},
			),
		)

		require.ErrorContains(t, err, `resource "database" entered FAILED`)
		assert.True(t, canceled, "a resource failure must cancel the processes")
	})

	t.Run("should shut the resources down when a process fails", func(t *testing.T) {
		errProcess := errors.New("process blew up")
		managed := resourcefixture.NewHealthyFakeManaged("database")

		err := app.Run(ctx,
			app.WithResource(managed),
			app.WithProcess(func(context.Context) error { return errProcess }),
		)

		require.ErrorIs(t, err, errProcess)
		assert.True(t, managed.WasShutdown())
	})

	t.Run("should shut the resources down and succeed on a POSIX signal", func(t *testing.T) {
		managed := resourcefixture.NewHealthyFakeManaged("database")

		err := app.Run(ctx,
			app.WithResource(managed),
			app.WithProcess(func(context.Context) error { return process.ErrPOSIXSignalReceived }),
		)

		require.NoError(t, err, "a POSIX signal is a graceful shutdown, not an error")
		assert.True(t, managed.WasShutdown())
	})

	t.Run("should succeed when the context is canceled while managing resources", func(t *testing.T) {
		managed := resourcefixture.NewHealthyFakeManaged("database")
		cancelCtx, cancel := context.WithCancel(ctx)

		result := make(chan error, 1)
		go func() {
			result <- app.Run(cancelCtx,
				app.WithResource(managed),
				app.WithProcess(func(processCtx context.Context) error { <-processCtx.Done(); return nil }),
			)
		}()

		cancel()

		err := athanorfixture.Receive(t, result, 5*time.Second, "Run did not return after the context was canceled")
		require.NoError(t, err, "a canceled context is a graceful shutdown, not an error")
		assert.True(t, managed.WasShutdown())
	})

	t.Run("should shut the resources down with the values of the context, bounded", func(t *testing.T) {
		type contextKey struct{}
		managed := resourcefixture.NewHealthyFakeManaged("database")
		cancelCtx, cancel := context.WithCancel(context.WithValue(ctx, contextKey{}, "trace-42"))
		cancel()

		err := app.Run(cancelCtx, app.WithResource(managed))

		require.NoError(t, err)
		shutdownCtx := managed.ShutdownContext()
		require.NotNil(t, shutdownCtx, "the resource was never shut down")
		require.NoError(t, managed.ShutdownContextError(), "a shutdown must not start with a canceled context")
		assert.Equal(t, "trace-42", shutdownCtx.Value(contextKey{}))
		_, hasDeadline := shutdownCtx.Deadline()
		assert.True(t, hasDeadline, "the shutdown must be bounded")
	})

	t.Run("should serve the registered HTTP handlers", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		port, result := athanorfixture.RunOnFreePort(t, func(port int) error {
			config := &httpserver.Config{BindInterface: "127.0.0.1", BindPort: port}
			// The HealthHandler reports on the List holding its own server, which runs as one composite resource.
			resources := resource.NewList()
			server := httpserver.NewServer(config, httpserver.NewHealthHandler(resources))
			if err := resources.Register(server); err != nil {
				return err
			}
			return app.Run(cancelCtx, app.WithResource(resources))
		})

		httpfixture.AssertEventuallyResponds(t, fmt.Sprintf("http://127.0.0.1:%d/health", port), http.StatusOK)

		cancel()
		err := athanorfixture.Receive(t, result, 5*time.Second, "Run did not return after the context was canceled")
		require.NoError(t, err)
	})

	t.Run("should consume messages with the registered NATS handlers", func(t *testing.T) {
		nc := natsfixture.ConnectToEmbeddedServer(t)
		handle := natsfixture.NewFakeHandlerFunc()
		handler := natsfixture.NewFakeHandler().WithHandleFunc("orders.created", "", handle.Handle)

		cancelCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		result := make(chan error, 1)
		go func() {
			result <- app.Run(cancelCtx,
				app.WithResource(natsserver.NewServer(nc, handler)),
			)
		}()

		require.Eventually(t, func() bool {
			return nc.NumSubscriptions() == 1
		}, 5*time.Second, 10*time.Millisecond, "the NATS server never subscribed")

		require.NoError(t, nc.Publish("orders.created", []byte("order")))
		requests := handle.WaitForRequests(t, 1)
		assert.Equal(t, []byte("order"), requests[0].Msg.Data)

		cancel()
		err := athanorfixture.Receive(t, result, 5*time.Second, "Run did not return after the context was canceled")
		require.NoError(t, err)
	})

	t.Run("should serve the registered gRPC handlers", func(t *testing.T) {
		listener := athanorfixture.ListenOnFreePort(t)
		// The HealthHandler reports on the List holding its own server, which runs as one composite resource.
		resources := resource.NewList()
		server := grpcserver.NewServer(&grpcserver.Config{}, grpcserver.NewHealthHandler(resources)).
			WithListener(listener)
		require.NoError(t, resources.Register(server))

		cancelCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		result := make(chan error, 1)
		go func() { result <- app.Run(cancelCtx, app.WithResource(resources)) }()

		grpcfixture.AssertEventuallyServing(t, grpcfixture.NewHealthClient(t, listener.Addr().String()), "")

		cancel()
		err := athanorfixture.Receive(t, result, 5*time.Second, "Run did not return after the context was canceled")
		require.NoError(t, err)
	})
}
