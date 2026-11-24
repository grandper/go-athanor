package fixture_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/fixture"
)

func TestReceive(t *testing.T) {
	t.Run("should return the value sent on the channel", func(t *testing.T) {
		ch := make(chan string, 1)
		ch <- "done"

		value := fixture.Receive(t, ch, time.Second, "nothing was sent")

		assert.Equal(t, "done", value)
	})

	t.Run("should wait for a value sent later", func(t *testing.T) {
		ch := make(chan int)
		go func() { ch <- 42 }()

		assert.Equal(t, 42, fixture.Receive(t, ch, time.Second, "nothing was sent"))
	})

	t.Run("should return at once when the channel is closed", func(t *testing.T) {
		done := make(chan struct{})
		close(done)

		fixture.Receive(t, done, time.Second, "the channel was never closed")
	})

	t.Run("fails the test with the message when nothing arrives in time", func(t *testing.T) {
		recorder := fixture.NewFailureRecorder()
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			fixture.Receive(recorder, make(chan error), 10*time.Millisecond,
				"Run did not return after %s", "the cancel")
		}()

		fixture.Receive(t, finished, time.Second, "Receive never gave up")
		require.Contains(t, recorder.Message(), "Run did not return after the cancel")
		assert.Contains(t, recorder.Message(), "10ms")
	})

	t.Run("should return the zero value when the failure does not stop the goroutine", func(t *testing.T) {
		recorder := fixture.NewFailureRecorder().ReturnFromFailNow()

		value := fixture.Receive(recorder, make(chan int), 10*time.Millisecond, "nothing was sent")

		assert.NotEmpty(t, recorder.Message())
		assert.Zero(t, value)
	})
}
