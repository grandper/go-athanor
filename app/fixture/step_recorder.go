// Package fixture provides reusable testing objects for the app package.
package fixture

import (
	"context"
	"sync"

	"github.com/grandper/go-athanor/app"
)

// StepRecorder records the hooks and processes that ran, in order. It is safe for concurrent use.
type StepRecorder struct {
	mu    sync.Mutex
	steps []string
}

// NewStepRecorder returns an empty StepRecorder.
func NewStepRecorder() *StepRecorder {
	return &StepRecorder{}
}

// Step returns a hook or process that records name when it runs and succeeds.
func (r *StepRecorder) Step(name string) app.ProcessFunc {
	return r.FailingStep(name, nil)
}

// FailingStep returns a hook or process that records name when it runs and returns err.
func (r *StepRecorder) FailingStep(name string, err error) app.ProcessFunc {
	return func(context.Context) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.steps = append(r.steps, name)
		return err
	}
}

// Steps returns a copy of the names recorded so far, in the order the steps ran.
func (r *StepRecorder) Steps() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.steps...)
}
