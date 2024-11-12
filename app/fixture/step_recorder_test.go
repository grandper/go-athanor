package fixture_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/app/fixture"
)

func TestStepRecorder(t *testing.T) {
	ctx := context.Background()

	t.Run("should record nothing before a step runs", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()

		_ = recorder.Step("migrate")

		assert.Empty(t, recorder.Steps())
	})

	t.Run("should record the steps in the order they ran", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()

		require.NoError(t, recorder.Step("migrate")(ctx))
		require.NoError(t, recorder.Step("serve")(ctx))

		assert.Equal(t, []string{"migrate", "serve"}, recorder.Steps())
	})

	t.Run("should record a failing step and return its error", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()

		err := recorder.FailingStep("migrate", assert.AnError)(ctx)

		require.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, []string{"migrate"}, recorder.Steps())
	})

	t.Run("should return a copy of the recorded steps", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()
		require.NoError(t, recorder.Step("migrate")(ctx))

		recorder.Steps()[0] = "tampered"

		assert.Equal(t, []string{"migrate"}, recorder.Steps())
	})

	t.Run("should record steps running concurrently", func(t *testing.T) {
		recorder := fixture.NewStepRecorder()
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = recorder.Step("poll")(ctx)
			}()
		}
		wg.Wait()

		assert.Len(t, recorder.Steps(), 50)
	})
}
