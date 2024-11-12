package resource_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	athanorfixture "github.com/grandper/go-athanor/fixture"
	"github.com/grandper/go-athanor/resource"
	"github.com/grandper/go-athanor/resource/fixture"
)

// statusOf reads a resource's current status through the List.
func statusOf(t *testing.T, c *resource.List, name string) resource.Status {
	t.Helper()
	obs, ok := c.StatusOf(name)
	require.True(t, ok, "resource %q not registered", name)
	return obs.Get()
}

func TestList(t *testing.T) {
	ctx := context.Background()

	t.Run("should reject a resource registered twice under the same name", func(t *testing.T) {
		c := resource.NewList()
		require.NoError(t, c.Register(fixture.NewFakeManaged("a")))

		err := c.Register(fixture.NewFakeManaged("a"))

		require.ErrorIs(t, err, resource.ErrAlreadyRegistered)
		require.ErrorContains(t, err, `"a"`)
		assert.Equal(t, resource.StatusInitializing, statusOf(t, c, "a"))
	})

	t.Run("should initialize a resource that sets its status synchronously from Init", func(t *testing.T) {
		// The List must not hold its lock across Init.
		c := resource.NewList()
		r := fixture.NewHealthyFakeManaged("nats")
		require.NoError(t, c.Register(r))

		done := make(chan error, 1)
		go func() { done <- c.Init(ctx) }()

		err := athanorfixture.Receive(t, done, 2*time.Second, "Init deadlocked on synchronous status change")
		require.NoError(t, err)

		assert.Equal(t, resource.StatusHealthy, statusOf(t, c, "nats"))
		assert.True(t, c.IsHealthy(), "expected list healthy")
	})

	t.Run("should roll back the already-started resources when a later Init fails", func(t *testing.T) {
		c := resource.NewList()
		good := fixture.NewHealthyFakeManaged("good")
		errBoom := errors.New("boom")
		bad := fixture.NewFakeManaged("bad").FailInitWith(errBoom)
		require.NoError(t, c.Register(good))
		require.NoError(t, c.Register(bad))

		require.ErrorIs(t, c.Init(ctx), errBoom)

		assert.Equal(t, resource.StatusFailed, statusOf(t, c, "bad"))
		assert.True(t, good.WasShutdown(), "expected previously-started resource to be shut down")
		assert.Equal(t, resource.StatusClosed, statusOf(t, c, "good"))
		assert.Equal(t, resource.StatusFailed, c.Status().Get())
	})

	t.Run("should roll back with a live context when the Init context is already canceled", func(t *testing.T) {
		c := resource.NewList()
		good := fixture.NewHealthyFakeManaged("good")
		bad := fixture.NewFakeManaged("bad").FailInitWith(errors.New("boom"))
		require.NoError(t, c.Register(good))
		require.NoError(t, c.Register(bad))
		canceledCtx, cancel := context.WithCancel(ctx)
		cancel()

		require.Error(t, c.Init(canceledCtx))

		require.True(t, good.WasShutdown())
		assert.NoError(t, good.ShutdownContextError(), "a rollback is a cleanup: it must not start canceled")
	})

	t.Run("fails to register a resource once Init was called", func(t *testing.T) {
		c := resource.NewList()
		require.NoError(t, c.Register(fixture.NewHealthyFakeManaged("database")))
		require.NoError(t, c.Init(ctx))

		err := c.Register(fixture.NewFakeManaged("late"))

		require.ErrorIs(t, err, resource.ErrListAlreadyInitialized)
		require.ErrorContains(t, err, `failed to register resource "late"`)
		_, registered := c.StatusOf("late")
		assert.False(t, registered, "a resource that would never be initialized must not join the List")
	})

	t.Run("should publish the aggregate of the latest statuses when resources change concurrently", func(t *testing.T) {
		// A refresh that publishes an aggregate computed before a concurrent change leaves it stale for good.
		for iteration := 0; iteration < 2000; iteration++ {
			c := resource.NewList()
			degrading := fixture.NewHealthyFakeManaged("degrading")
			failing := fixture.NewHealthyFakeManaged("failing")
			require.NoError(t, c.Register(degrading))
			require.NoError(t, c.Register(failing))
			require.NoError(t, c.Init(ctx))

			var group sync.WaitGroup
			group.Add(2)
			go func() { defer group.Done(); degrading.Notify(resource.StatusDegraded) }()
			go func() { defer group.Done(); failing.Notify(resource.StatusUnhealthy) }()
			group.Wait()

			require.Equal(t, resource.StatusUnhealthy, c.Status().Get(), "stale aggregate at iteration %d", iteration)
		}
	})

	t.Run("should let an observer of the aggregate change a resource without deadlocking", func(t *testing.T) {
		c := resource.NewList()
		first := fixture.NewHealthyFakeManaged("first")
		second := fixture.NewHealthyFakeManaged("second")
		require.NoError(t, c.Register(first))
		require.NoError(t, c.Register(second))
		require.NoError(t, c.Init(ctx))
		// Observers run synchronously inside the refresh, so this one re-enters it.
		unsubscribe := c.Status().Observe(resource.StatusObserverFunc(func(s resource.Status) {
			if s == resource.StatusDegraded {
				second.Notify(resource.StatusUnhealthy)
			}
		}))
		defer unsubscribe()

		done := make(chan struct{})
		go func() { defer close(done); first.Notify(resource.StatusDegraded) }()

		athanorfixture.Receive(t, done, 2*time.Second, "the refresh deadlocked on a re-entrant status change")
		assert.Equal(t, resource.StatusUnhealthy, c.Status().Get())
	})

	t.Run("should return the failure cause from Manage when a resource fails", func(t *testing.T) {
		c := resource.NewList()
		r := fixture.NewHealthyFakeManaged("nats")
		require.NoError(t, c.Register(r))
		require.NoError(t, c.Init(ctx))

		errCh := make(chan error, 1)
		go func() { errCh <- c.Manage(ctx) }()

		// Manage waits on signals that stay raised, so the outcome is the same whether or not it already blocks.
		r.Notify(resource.StatusFailed)

		err := athanorfixture.Receive(t, errCh, 2*time.Second, "Manage did not return after StatusFailed")
		require.ErrorIs(t, err, resource.ErrResourceFailed)
		require.ErrorContains(t, err, `"nats"`)

		assert.Equal(t, resource.StatusFailed, statusOf(t, c, "nats"))
		assert.Equal(t, resource.StatusFailed, c.Status().Get())
	})

	t.Run("should return from Manage when the context is canceled", func(t *testing.T) {
		c := resource.NewList()
		r := fixture.NewHealthyFakeManaged("nats")
		require.NoError(t, c.Register(r))
		require.NoError(t, c.Init(ctx))

		cancelCtx, cancel := context.WithCancel(ctx)
		errCh := make(chan error, 1)
		go func() { errCh <- c.Manage(cancelCtx) }()

		cancel()

		err := athanorfixture.Receive(t, errCh, 2*time.Second, "Manage did not return after context cancel")
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("should return from Manage on context cancellation when no resource is registered", func(t *testing.T) {
		c := resource.NewList()
		cancelCtx, cancel := context.WithCancel(ctx)
		errCh := make(chan error, 1)
		go func() { errCh <- c.Manage(cancelCtx) }()
		cancel()
		err := athanorfixture.Receive(t, errCh, time.Second, "Manage with no resources did not return")
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("should not report a failure to Manage on a clean Shutdown", func(t *testing.T) {
		c := resource.NewList()
		r := fixture.NewHealthyFakeManaged("nats")
		require.NoError(t, c.Register(r))
		require.NoError(t, c.Init(ctx))

		errCh := make(chan error, 1)
		go func() { errCh <- c.Manage(ctx) }()

		require.NoError(t, c.Shutdown(ctx))
		assert.True(t, r.WasShutdown(), "resource was not shut down")
		assert.Equal(t, resource.StatusClosed, statusOf(t, c, "nats"))
		assert.Equal(t, resource.StatusClosed, c.Status().Get())

		err := athanorfixture.Receive(t, errCh, 2*time.Second, "Manage did not return after Shutdown")
		require.NoError(t, err, "Manage should return nil after a clean Shutdown")
	})

	t.Run("should stay in StatusInitializing when it is empty, even once shut down", func(t *testing.T) {
		c := resource.NewList()
		require.NoError(t, c.Init(ctx))

		require.NoError(t, c.Shutdown(ctx))

		assert.Equal(t, resource.StatusInitializing, c.Status().Get())
	})

	t.Run("should shut down every resource and return the first error when a Shutdown fails", func(t *testing.T) {
		c := resource.NewList()
		errFirst := errors.New("first is stuck")
		errLast := errors.New("last is stuck")
		first := fixture.NewHealthyFakeManaged("first").FailShutdownWith(errFirst)
		last := fixture.NewHealthyFakeManaged("last").FailShutdownWith(errLast)
		require.NoError(t, c.Register(first))
		require.NoError(t, c.Register(last))
		require.NoError(t, c.Init(ctx))

		err := c.Shutdown(ctx)

		// Resources shut down in reverse order, so the last one fails first.
		require.ErrorIs(t, err, errLast)
		require.ErrorContains(t, err, `failed to shut down "last"`)
		assert.True(t, first.WasShutdown(), "a failing Shutdown must not stop the others")
		assert.True(t, last.WasShutdown())
	})

	t.Run("should report health and per-resource statuses", func(t *testing.T) {
		c := resource.NewList()
		assert.True(t, c.IsHealthy(), "empty list should be healthy")

		a := fixture.NewHealthyFakeManaged("a")
		b := fixture.NewHealthyFakeManaged("b")
		require.NoError(t, c.Register(a))
		require.NoError(t, c.Register(b))
		require.NoError(t, c.Init(ctx))
		assert.True(t, c.IsHealthy(), "all healthy should be healthy")

		b.Notify(resource.StatusDegraded)
		assert.True(t, c.IsHealthy(), "degraded is still operational")

		b.Notify(resource.StatusUnhealthy)
		assert.False(t, c.IsHealthy(), "unhealthy should make the list unhealthy")

		sum := c.StatusSummary()
		assert.Equal(t, resource.StatusHealthy, sum["a"])
		assert.Equal(t, resource.StatusUnhealthy, sum["b"])

		sum["a"] = resource.StatusFailed
		assert.Equal(t, resource.StatusHealthy, statusOf(t, c, "a"), "StatusSummary must return a copy")
	})

	t.Run("should expose one resource's status through StatusOf", func(t *testing.T) {
		c := resource.NewList()
		r := fixture.NewHealthyFakeManaged("nats")
		require.NoError(t, c.Register(r))
		require.NoError(t, c.Init(ctx))

		obs, ok := c.StatusOf("nats")
		require.True(t, ok, "StatusOf(nats) not found")
		assert.Equal(t, resource.StatusHealthy, obs.Get())

		changed := make(chan resource.Status, 1)
		obs.Observe(resource.StatusObserverFunc(func(s resource.Status) { changed <- s }))
		r.Notify(resource.StatusDegraded)
		s := athanorfixture.Receive(t, changed, time.Second, "external observer was not notified")
		assert.Equal(t, resource.StatusDegraded, s)

		_, found := c.StatusOf("missing")
		assert.False(t, found, "StatusOf(missing) should report not found")
	})

	t.Run("should aggregate the resources' statuses into its own observable status", func(t *testing.T) {
		c := resource.NewList()
		assert.Equal(t, resource.StatusInitializing, c.Status().Get(), "empty list should be INITIALIZING")

		a := fixture.NewHealthyFakeManaged("a")
		b := fixture.NewHealthyFakeManaged("b")
		require.NoError(t, c.Register(a))
		require.NoError(t, c.Register(b))
		assert.Equal(t, resource.StatusInitializing, c.Status().Get(), "registered but not initialized")

		var seen []resource.Status
		c.Status().Observe(resource.StatusObserverFunc(func(s resource.Status) { seen = append(seen, s) }))

		require.NoError(t, c.Init(ctx))
		assert.Equal(t, resource.StatusHealthy, c.Status().Get(), "all resources healthy after Init")

		b.Notify(resource.StatusDegraded)
		assert.Equal(t, resource.StatusDegraded, c.Status().Get())

		a.Notify(resource.StatusUnhealthy)
		assert.Equal(t, resource.StatusUnhealthy, c.Status().Get())

		a.Notify(resource.StatusHealthy)
		b.Notify(resource.StatusHealthy)
		assert.Equal(t, resource.StatusHealthy, c.Status().Get())

		require.NoError(t, c.Shutdown(ctx))
		assert.Equal(t, resource.StatusClosed, c.Status().Get(), "all resources closed after Shutdown")

		// Intermediate DEGRADED steps come from partial recovery and partial shutdown.
		want := []resource.Status{
			resource.StatusHealthy, resource.StatusDegraded, resource.StatusUnhealthy,
			resource.StatusDegraded, resource.StatusHealthy, resource.StatusDegraded,
			resource.StatusClosed,
		}
		assert.Equal(t, want, seen)
	})

	t.Run("should nest inside another List as a composite resource", func(t *testing.T) {
		inner := resource.NewList()
		r := fixture.NewHealthyFakeManaged("db")
		require.NoError(t, inner.Register(r))

		outer := resource.NewList()
		require.NoError(t, outer.Register(inner))
		require.NoError(t, outer.Init(ctx))
		assert.Equal(t, resource.StatusHealthy, outer.Status().Get())

		r.Notify(resource.StatusFailed)
		assert.Equal(t, resource.StatusFailed, outer.Status().Get())
	})
}
