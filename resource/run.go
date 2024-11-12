package resource

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Run initializes the resource and blocks until ctx is canceled or the resource reports StatusFailed, then
// shuts the resource down with a CleanupContext. A canceled context is a graceful stop, not an error: Run
// fails when the resource cannot be initialized, when it failed, wrapping ErrResourceFailed, or when it
// cannot be shut down.
func Run(ctx context.Context, managed Managed) error {
	if err := managed.Init(ctx); err != nil {
		return err
	}

	failed := make(chan struct{})
	var once sync.Once
	notifyFailure := func() { once.Do(func() { close(failed) }) }
	unsubscribe := managed.Status().Observe(StatusObserverFunc(func(s Status) {
		if s.IsFailed() {
			notifyFailure()
		}
	}))
	defer unsubscribe()
	// Observe does not replay the current status, so an earlier failure is checked explicitly.
	if managed.Status().Get().IsFailed() {
		notifyFailure()
	}

	var failure error
	select {
	case <-ctx.Done():
	case <-failed:
		failure = fmt.Errorf("resource %q failed: %w", managed.Name(), ErrResourceFailed)
	}

	cleanupCtx, cancel := CleanupContext(ctx)
	defer cancel()
	return errors.Join(failure, managed.Shutdown(cleanupCtx))
}
