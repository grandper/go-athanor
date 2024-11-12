package fixture_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource"
	"github.com/grandper/go-athanor/resource/fixture"
)

func TestFakeManaged(t *testing.T) {
	ctx := context.Background()

	t.Run("should start in StatusInitializing with the given name", func(t *testing.T) {
		f := fixture.NewFakeManaged("db")
		assert.Equal(t, "db", f.Name())
		assert.Equal(t, resource.StatusInitializing, f.Status().Get())
		assert.False(t, f.WasShutdown())
	})

	t.Run("should stay in StatusInitializing after Init by default", func(t *testing.T) {
		f := fixture.NewFakeManaged("db")
		require.NoError(t, f.Init(ctx))
		assert.Equal(t, resource.StatusInitializing, f.Status().Get())
	})

	t.Run("should report StatusHealthy from Init when built as healthy", func(t *testing.T) {
		f := fixture.NewHealthyFakeManaged("db")
		require.NoError(t, f.Init(ctx))
		assert.Equal(t, resource.StatusHealthy, f.Status().Get())
	})

	t.Run("fails Init with the configured error", func(t *testing.T) {
		errInit := errors.New("boom")
		f := fixture.NewFakeManaged("db").FailInitWith(errInit)

		require.ErrorIs(t, f.Init(ctx), errInit)
		assert.Equal(t, resource.StatusFailed, f.Status().Get())
	})

	t.Run("should record a successful shutdown and report StatusClosed", func(t *testing.T) {
		f := fixture.NewHealthyFakeManaged("db")
		require.NoError(t, f.Shutdown(ctx))
		assert.True(t, f.WasShutdown())
		assert.Equal(t, resource.StatusClosed, f.Status().Get())
	})

	t.Run("fails Shutdown with the configured error", func(t *testing.T) {
		errShutdown := errors.New("boom")
		f := fixture.NewFakeManaged("db").FailShutdownWith(errShutdown)

		require.ErrorIs(t, f.Shutdown(ctx), errShutdown)
		assert.True(t, f.WasShutdown())
		assert.Equal(t, resource.StatusFailed, f.Status().Get())
	})

	t.Run("should record the state of the context Shutdown was called with", func(t *testing.T) {
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()
		live := fixture.NewFakeManaged("live")
		canceled := fixture.NewFakeManaged("canceled")

		require.NoError(t, live.Shutdown(context.Background()))
		require.NoError(t, canceled.Shutdown(canceledCtx))

		require.NoError(t, live.ShutdownContextError())
		assert.ErrorIs(t, canceled.ShutdownContextError(), context.Canceled)
	})

	t.Run("should record the context Shutdown was called with", func(t *testing.T) {
		type contextKey struct{}
		f := fixture.NewFakeManaged("db")
		assert.Nil(t, f.ShutdownContext(), "no context before Shutdown")

		require.NoError(t, f.Shutdown(context.WithValue(context.Background(), contextKey{}, "trace-42")))

		require.NotNil(t, f.ShutdownContext())
		assert.Equal(t, "trace-42", f.ShutdownContext().Value(contextKey{}))
	})

	t.Run("should push status changes through Notify", func(t *testing.T) {
		f := fixture.NewFakeManaged("db")
		f.Notify(resource.StatusDegraded)
		assert.Equal(t, resource.StatusDegraded, f.Status().Get())
	})
}
