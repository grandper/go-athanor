package app_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/app"
	"github.com/grandper/go-athanor/app/fixture"
	"github.com/grandper/go-athanor/resource"
	resourcefixture "github.com/grandper/go-athanor/resource/fixture"
)

func TestWithHookBefore(t *testing.T) {
	ctx := context.Background()

	t.Run("should run the hook before the processes start", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()

		err := app.Run(ctx,
			app.WithProcess(recorder.Step("process")),
			app.WithHookBefore(recorder.Step("before")),
		)

		require.NoError(t, err)
		assert.Equal(t, []string{"before", "process"}, recorder.Steps())
	})

	t.Run("fails without starting the processes when the hook fails", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()

		err := app.Run(ctx,
			app.WithHookBefore(recorder.FailingStep("before", assert.AnError)),
			app.WithProcess(recorder.Step("process")),
		)

		require.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, []string{"before"}, recorder.Steps())
	})
}

func TestWithHooksBefore(t *testing.T) {
	ctx := context.Background()

	t.Run("should run the hooks sequentially in the given order", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()

		err := app.Run(ctx,
			app.WithHooksBefore(recorder.Step("before-1"), recorder.Step("before-2")),
			app.WithHookBefore(recorder.Step("before-3")),
		)

		require.NoError(t, err)
		assert.Equal(t, []string{"before-1", "before-2", "before-3"}, recorder.Steps())
	})

	t.Run("fails without running the following hooks when a hook fails", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()

		err := app.Run(ctx,
			app.WithHooksBefore(recorder.FailingStep("before-1", assert.AnError), recorder.Step("before-2")),
		)

		require.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, []string{"before-1"}, recorder.Steps())
	})
}

func TestWithHookAfter(t *testing.T) {
	ctx := context.Background()

	t.Run("should run the hook once the processes have stopped", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()

		err := app.Run(ctx,
			app.WithHookAfter(recorder.Step("after")),
			app.WithProcess(recorder.Step("process")),
		)

		require.NoError(t, err)
		assert.Equal(t, []string{"process", "after"}, recorder.Steps())
	})

	t.Run("should run the hook even when a process fails", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()

		err := app.Run(ctx,
			app.WithProcess(recorder.FailingStep("process", assert.AnError)),
			app.WithHookAfter(recorder.Step("after")),
		)

		require.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, []string{"process", "after"}, recorder.Steps())
	})

	t.Run("fails when the hook fails", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()

		err := app.Run(ctx, app.WithHookAfter(recorder.FailingStep("after", assert.AnError)))

		require.ErrorIs(t, err, assert.AnError)
	})
}

func TestWithHooksAfter(t *testing.T) {
	ctx := context.Background()

	t.Run("should run the hooks sequentially in the given order", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()

		err := app.Run(ctx,
			app.WithHooksAfter(recorder.Step("after-1"), recorder.Step("after-2")),
			app.WithHookAfter(recorder.Step("after-3")),
		)

		require.NoError(t, err)
		assert.Equal(t, []string{"after-1", "after-2", "after-3"}, recorder.Steps())
	})
}

func TestWithProcess(t *testing.T) {
	ctx := context.Background()

	t.Run("should run the process", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()

		err := app.Run(ctx, app.WithProcess(recorder.Step("process")))

		require.NoError(t, err)
		assert.Equal(t, []string{"process"}, recorder.Steps())
	})

	t.Run("fails with the error of the process", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()

		err := app.Run(ctx, app.WithProcess(recorder.FailingStep("process", assert.AnError)))

		require.ErrorIs(t, err, assert.AnError)
	})
}

func TestWithProcesses(t *testing.T) {
	ctx := context.Background()

	t.Run("should run every process", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()

		err := app.Run(ctx,
			app.WithProcesses(recorder.Step("poll"), recorder.Step("sweep")),
			app.WithProcess(recorder.Step("sync")),
		)

		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"poll", "sweep", "sync"}, recorder.Steps())
	})

	t.Run("should cancel the other processes when one of them fails", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()
		waitForCancellation := func(ctx context.Context) error {
			<-ctx.Done()
			return recorder.Step("canceled")(ctx)
		}

		err := app.Run(ctx,
			app.WithProcesses(waitForCancellation, recorder.FailingStep("failing", assert.AnError)),
		)

		require.ErrorIs(t, err, assert.AnError)
		assert.ElementsMatch(t, []string{"failing", "canceled"}, recorder.Steps())
	})
}

func TestWithResource(t *testing.T) {
	ctx := context.Background()

	t.Run("should initialize and shut down the registered resource", func(t *testing.T) {
		managed := resourcefixture.NewHealthyFakeManaged("database")
		// The canceled context ends the run once the resource is initialized.
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		err := app.Run(cancelCtx, app.WithResource(managed))

		require.NoError(t, err)
		assert.True(t, managed.WasShutdown())
		assert.Equal(t, resource.StatusClosed, managed.Status().Get())
	})

	t.Run("fails when two resources share the same name", func(t *testing.T) {
		err := app.Run(ctx,
			app.WithResource(resourcefixture.NewHealthyFakeManaged("database")),
			app.WithResource(resourcefixture.NewHealthyFakeManaged("database")),
		)

		require.ErrorIs(t, err, resource.ErrAlreadyRegistered)
		require.ErrorContains(t, err, `"database"`)
	})
}

func TestWithResources(t *testing.T) {
	ctx := context.Background()

	t.Run("should initialize and shut down every registered resource", func(t *testing.T) {
		first := resourcefixture.NewHealthyFakeManaged("database")
		second := resourcefixture.NewHealthyFakeManaged("cache")
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		err := app.Run(cancelCtx, app.WithResources(first, second))

		require.NoError(t, err)
		assert.True(t, first.WasShutdown())
		assert.True(t, second.WasShutdown())
	})

	t.Run("fails when one of the resources cannot be registered", func(t *testing.T) {
		err := app.Run(ctx,
			app.WithResources(
				resourcefixture.NewHealthyFakeManaged("database"),
				resourcefixture.NewHealthyFakeManaged("database"),
			),
		)

		require.ErrorIs(t, err, resource.ErrAlreadyRegistered)
		require.ErrorContains(t, err, `"database"`)
	})
}
