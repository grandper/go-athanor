// Package fixture provides reusable testing objects for the resource package.
package fixture

import (
	"context"
	"sync"

	"github.com/grandper/go-athanor/resource"
)

// FakeManaged is a controllable resource.Managed implementation for tests.
type FakeManaged struct {
	name       string
	initStatus resource.Status // status reported from Init
	reportInit bool
	initErr    error
	shutErr    error

	status *resource.ObservableStatus

	mu             sync.Mutex
	shutdown       bool
	shutdownCtx    context.Context
	shutdownCtxErr error
}

// NewFakeManaged creates a fake resource that stays in StatusInitializing.
func NewFakeManaged(name string) *FakeManaged {
	return &FakeManaged{
		name:   name,
		status: resource.NewObservableStatus(resource.StatusInitializing),
	}
}

// NewHealthyFakeManaged creates a fake resource that reports StatusHealthy from Init.
func NewHealthyFakeManaged(name string) *FakeManaged {
	f := NewFakeManaged(name)
	f.reportInit = true
	f.initStatus = resource.StatusHealthy
	return f
}

// FailInitWith makes Init report StatusFailed and fail with err.
func (f *FakeManaged) FailInitWith(err error) *FakeManaged {
	f.initErr = err
	return f
}

// FailShutdownWith makes Shutdown report StatusFailed and fail with err.
func (f *FakeManaged) FailShutdownWith(err error) *FakeManaged {
	f.shutErr = err
	return f
}

// Name implements the resource.Managed interface.
func (f *FakeManaged) Name() string { return f.name }

// Status implements the resource.Managed interface.
func (f *FakeManaged) Status() *resource.ObservableStatus { return f.status }

// Init reports the configured status, or StatusFailed with the configured error.
func (f *FakeManaged) Init(_ context.Context) error {
	if f.initErr != nil {
		f.status.Set(resource.StatusFailed)
		return f.initErr
	}
	if f.reportInit {
		f.status.Set(f.initStatus)
	}
	return nil
}

// Shutdown records the call, ctx, and its state, and reports StatusClosed, or StatusFailed with the configured error.
func (f *FakeManaged) Shutdown(ctx context.Context) error {
	f.mu.Lock()
	f.shutdown = true
	f.shutdownCtx = ctx
	f.shutdownCtxErr = ctx.Err()
	f.mu.Unlock()

	if f.shutErr != nil {
		f.status.Set(resource.StatusFailed)
		return f.shutErr
	}
	f.status.Set(resource.StatusClosed)
	return nil
}

// Notify sets the status of the resource.
func (f *FakeManaged) Notify(s resource.Status) { f.status.Set(s) }

// WasShutdown reports whether Shutdown was called.
func (f *FakeManaged) WasShutdown() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.shutdown
}

// ShutdownContext returns the context Shutdown was called with, or nil when it was not called: use it to
// check the values and the deadline a cleanup was handed.
func (f *FakeManaged) ShutdownContext() context.Context {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.shutdownCtx
}

// ShutdownContextError returns the error of the context at the time Shutdown was called with it:
// nil for a live context, context.Canceled or context.DeadlineExceeded otherwise.
func (f *FakeManaged) ShutdownContextError() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.shutdownCtxErr
}

// FakeManaged implements the resource.Managed interface.
var _ resource.Managed = (*FakeManaged)(nil)
