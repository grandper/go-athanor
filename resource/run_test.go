package resource_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	athanorfixture "github.com/grandper/go-athanor/fixture"
	"github.com/grandper/go-athanor/resource"
	"github.com/grandper/go-athanor/resource/fixture"
)

func TestRun(t *testing.T) {
	ctx := context.Background()

	t.Run("fails when the resource cannot be initialized, without shutting it down", func(t *testing.T) {
		errInit := errors.New("init failed")
		managed := fixture.NewFakeManaged("database").FailInitWith(errInit)

		err := resource.Run(ctx, managed)

		require.ErrorIs(t, err, errInit)
		assert.False(t, managed.WasShutdown())
	})

	t.Run("should shut the resource down when the context is canceled", func(t *testing.T) {
		managed := fixture.NewHealthyFakeManaged("database")
		cancelCtx, cancel := context.WithCancel(ctx)
		result := make(chan error, 1)
		go func() { result <- resource.Run(cancelCtx, managed) }()
		require.Never(t, managed.WasShutdown, 50*time.Millisecond, 5*time.Millisecond,
			"the resource must run until the context is canceled")

		cancel()

		err := athanorfixture.Receive(t, result, 5*time.Second, "Run did not return after the context was canceled")
		require.NoError(t, err, "a canceled context is a graceful shutdown, not an error")
		assert.True(t, managed.WasShutdown())
	})

	t.Run("should shut the resource down with the values of the context, bounded", func(t *testing.T) {
		type contextKey struct{}
		managed := fixture.NewHealthyFakeManaged("database")
		cancelCtx, cancel := context.WithCancel(context.WithValue(ctx, contextKey{}, "trace-42"))
		cancel()

		require.NoError(t, resource.Run(cancelCtx, managed))

		shutdownCtx := managed.ShutdownContext()
		require.NotNil(t, shutdownCtx, "the resource was never shut down")
		require.NoError(t, managed.ShutdownContextError(), "a shutdown must not start with a canceled context")
		assert.Equal(t, "trace-42", shutdownCtx.Value(contextKey{}))
		_, hasDeadline := shutdownCtx.Deadline()
		assert.True(t, hasDeadline, "the shutdown must be bounded")
	})

	t.Run("fails when the resource reports StatusFailed, after shutting it down", func(t *testing.T) {
		managed := fixture.NewHealthyFakeManaged("database")
		result := make(chan error, 1)
		go func() { result <- resource.Run(ctx, managed) }()

		// Notify until Run observes it: the failure may come before Run subscribed.
		require.Eventually(t, func() bool {
			managed.Notify(resource.StatusFailed)
			return managed.WasShutdown()
		}, 5*time.Second, 5*time.Millisecond, "the failure never stopped Run")

		err := athanorfixture.Receive(t, result, 5*time.Second, "Run did not return after the resource failed")
		require.ErrorIs(t, err, resource.ErrResourceFailed)
		require.ErrorContains(t, err, `resource "database" failed`)
	})

	t.Run("fails when the resource already failed once initialized", func(t *testing.T) {
		managed := fixture.NewFakeManaged("database")
		managed.Notify(resource.StatusFailed)

		err := resource.Run(ctx, managed)

		require.ErrorIs(t, err, resource.ErrResourceFailed)
		assert.True(t, managed.WasShutdown())
	})

	t.Run("fails when the resource cannot be shut down", func(t *testing.T) {
		errShutdown := errors.New("shutdown failed")
		managed := fixture.NewHealthyFakeManaged("database").FailShutdownWith(errShutdown)
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		err := resource.Run(cancelCtx, managed)

		require.ErrorIs(t, err, errShutdown)
		assert.NotErrorIs(t, err, resource.ErrResourceFailed)
	})
}
