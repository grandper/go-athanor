package fixture_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/fixture"
)

func TestFailureRecorder(t *testing.T) {
	t.Run("should not be failed before any assertion fails", func(t *testing.T) {
		recorder := fixture.NewFailureRecorder()

		assert.False(t, recorder.Failed())
		assert.Empty(t, recorder.Message())
	})

	t.Run("should record a failed assert without stopping the goroutine", func(t *testing.T) {
		recorder := fixture.NewFailureRecorder()

		passed := assert.Equal(recorder, "expected", "actual", "the values differ")

		assert.False(t, passed)
		assert.True(t, recorder.Failed())
		assert.Contains(t, recorder.Message(), "the values differ")
	})

	t.Run("should record a failed require and stop the goroutine like testing.T", func(t *testing.T) {
		recorder := fixture.NewFailureRecorder()
		reachedAfterFailure := false
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			fixture.Receive(recorder, make(chan int), 10*time.Millisecond, "failed after %d attempts", 3)
			reachedAfterFailure = true
		}()

		fixture.Receive(t, finished, time.Second, "the failed require never stopped the goroutine")
		assert.True(t, recorder.Failed())
		assert.Contains(t, recorder.Message(), "failed after 3 attempts")
		assert.False(t, reachedAfterFailure, "a failed require must stop the calling goroutine")
	})

	t.Run("should return from a failed require when configured to", func(t *testing.T) {
		recorder := fixture.NewFailureRecorder().ReturnFromFailNow()

		require.Fail(recorder, "boom")

		assert.True(t, recorder.Failed())
		assert.Contains(t, recorder.Message(), "boom")
	})

	t.Run("implements the TestingT interface", func(t *testing.T) {
		assert.Implements(t, (*fixture.TestingT)(nil), fixture.NewFailureRecorder())
	})
}
