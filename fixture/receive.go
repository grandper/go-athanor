package fixture

import (
	"fmt"
	"time"
)

// TestingT is the subset of *testing.T needed by the helpers that fail the test on their own.
type TestingT interface {
	Helper()
	Fatalf(format string, args ...any)
}

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
		t.Fatalf("%s (nothing received within %s)", fmt.Sprintf(format, args...), timeout)
	}

	// Only reached with a TestingT whose Fatalf returns.
	var zero T
	return zero
}
