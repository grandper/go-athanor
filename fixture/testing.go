package fixture

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/stretchr/testify/require"
)

// TestingT is the subset of *testing.T needed by the helpers that fail the test on their own with testify.
type TestingT interface {
	require.TestingT
	Helper()
}

// FailureRecorder is a TestingT recording the failures reported by testify instead of failing the running test,
// so that tests can check how a helper fails.
type FailureRecorder struct {
	mu       sync.Mutex
	messages []string
	failed   bool
	returns  bool // makes FailNow return instead of stopping the goroutine
}

// NewFailureRecorder returns a FailureRecorder whose FailNow stops the calling goroutine, as testing.T does.
// Call the helper under test from its own goroutine.
func NewFailureRecorder() *FailureRecorder {
	return &FailureRecorder{}
}

// ReturnFromFailNow makes FailNow return instead of stopping the goroutine, to reach the code that follows it.
func (r *FailureRecorder) ReturnFromFailNow() *FailureRecorder {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.returns = true
	return r
}

// Helper implements TestingT and does nothing.
func (r *FailureRecorder) Helper() {}

// Errorf records the formatted failure message and marks the recorder as failed.
func (r *FailureRecorder) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
	r.failed = true
}

// FailNow marks the recorder as failed, then stops the calling goroutine unless configured to return.
func (r *FailureRecorder) FailNow() {
	r.mu.Lock()
	r.failed = true
	returns := r.returns
	r.mu.Unlock()

	if !returns {
		runtime.Goexit()
	}
}

// Failed reports whether a failure was recorded.
func (r *FailureRecorder) Failed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failed
}

// Message returns every recorded failure message, one per line.
func (r *FailureRecorder) Message() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.messages, "\n")
}

// FailureRecorder implements the TestingT interface.
var _ TestingT = (*FailureRecorder)(nil)
