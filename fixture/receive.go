package fixture

import (
	"fmt"
	"time"

	"github.com/stretchr/testify/require"
)

// Receive returns the next value received from ch, or fails the test with the formatted message when nothing
// arrives within timeout. A closed channel counts as an arrival, so it also waits for a done channel.
func Receive[T any](t TestingT, ch <-chan T, timeout time.Duration, format string, args ...any) T {
	t.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case value := <-ch:
		return value
	case <-timer.C:
		require.FailNowf(t, fmt.Sprintf(format, args...), "nothing received within %s", timeout)
	}

	// Only reached with a TestingT whose FailNow returns.
	var zero T
	return zero
}
