package resource

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrAlreadyRegistered is returned when a resource is registered under a name that is already taken.
	ErrAlreadyRegistered = errors.New("resource already registered")

	// ErrListAlreadyInitialized is returned when a resource is registered after Init was called:
	// the resource would never be initialized nor observed.
	ErrListAlreadyInitialized = errors.New("resource list already initialized")

	// ErrResourceFailed is returned by Manage when a resource reports StatusFailed.
	ErrResourceFailed = errors.New("resource failed")
)

// List manages the lifecycle of resources and is itself a [Managed] resource with an aggregate status.
type List struct {
	mu          sync.RWMutex
	resources   map[string]Managed
	order       []string          // registration order
	unsubscribe map[string]func() // per-resource observer removal
	initialized bool              // set by Init, which closes the registration

	// status is the aggregate status, written only by refreshStatus.
	status *ObservableStatus

	// refreshMu guards the two flags that let a single goroutine at a time publish the aggregate.
	refreshMu      sync.Mutex
	refreshing     bool // a goroutine is publishing the aggregate
	refreshPending bool // statuses may have changed since the publisher last read them

	// failedCtx is canceled the first time a resource reports StatusFailed.
	failedCtx    context.Context
	failedCancel context.CancelCauseFunc
}

// NewList returns an empty List ready for [List.Register].
func NewList() *List {
	failedCtx, failedCancel := context.WithCancelCause(context.Background())
	return &List{
		resources:    make(map[string]Managed),
		unsubscribe:  make(map[string]func()),
		status:       NewObservableStatus(StatusInitializing),
		failedCtx:    failedCtx,
		failedCancel: failedCancel,
	}
}

// Name returns the name of the List.
func (l *List) Name() string {
	return "resources"
}

// Register adds a managed resource to the List, failing if its name is already registered.
func (l *List) Register(r Managed) error {
	name := r.Name()

	l.mu.Lock()
	if l.initialized {
		l.mu.Unlock()
		return fmt.Errorf("failed to register resource %q: %w", name, ErrListAlreadyInitialized)
	}
	if _, exists := l.resources[name]; exists {
		l.mu.Unlock()
		return fmt.Errorf("failed to register resource %q: %w", name, ErrAlreadyRegistered)
	}
	l.resources[name] = r
	l.order = append(l.order, name)
	l.mu.Unlock()

	l.refreshStatus()
	return nil
}

// Init initializes all resources in registration order, rolling back the started ones on failure.
func (l *List) Init(ctx context.Context) error {
	l.mu.Lock()
	l.initialized = true
	order := append([]string(nil), l.order...)
	l.mu.Unlock()

	var started []string

	for _, name := range order {
		resourceName := name

		l.mu.RLock()
		res := l.resources[resourceName]
		l.mu.RUnlock()

		// Subscribe before Init to catch status changes emitted during Init.
		unsub := res.Status().Observe(StatusObserverFunc(func(s Status) {
			if s.IsFailed() {
				l.failedCancel(fmt.Errorf("resource %q entered %s: %w", resourceName, s, ErrResourceFailed))
			}
			l.refreshStatus()
		}))

		if err := res.Init(ctx); err != nil {
			unsub()
			l.rollback(ctx, started)
			return fmt.Errorf("failed to init resource %q: %w", resourceName, err)
		}

		l.mu.Lock()
		l.unsubscribe[resourceName] = unsub
		l.mu.Unlock()
		started = append(started, resourceName)
	}

	return nil
}

// rollback shuts down the started resources in reverse order. It is a cleanup, so it keeps the values of
// ctx but not its cancellation, which is often the very reason Init failed.
func (l *List) rollback(ctx context.Context, started []string) {
	ctx, cancel := CleanupContext(ctx)
	defer cancel()

	for i := len(started) - 1; i >= 0; i-- {
		name := started[i]

		l.mu.Lock()
		if unsub, ok := l.unsubscribe[name]; ok {
			unsub()
			delete(l.unsubscribe, name)
		}
		res := l.resources[name]
		l.mu.Unlock()

		_ = res.Shutdown(ctx)
	}

	l.refreshStatus()
}

// Manage blocks until a resource fails, returning the failure cause, or until ctx is canceled.
func (l *List) Manage(ctx context.Context) error {
	l.mu.RLock()
	failedCtx := l.failedCtx
	l.mu.RUnlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-failedCtx.Done():
		cause := context.Cause(failedCtx)
		if cause != nil && !errors.Is(cause, context.Canceled) {
			return cause
		}
		// A clean shutdown is not a failure.
		return ctx.Err()
	}
}

// Shutdown shuts down all resources in reverse registration order and returns the first error.
func (l *List) Shutdown(ctx context.Context) error {
	l.mu.RLock()
	order := append([]string(nil), l.order...)
	resources := make(map[string]Managed, len(l.resources))
	for name, res := range l.resources {
		resources[name] = res
	}
	unsubs := make([]func(), 0, len(l.unsubscribe))
	for _, unsub := range l.unsubscribe {
		unsubs = append(unsubs, unsub)
	}
	l.mu.RUnlock()

	var errs []error
	for i := len(order) - 1; i >= 0; i-- {
		name := order[i]
		if err := resources[name].Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shut down %q: %w", name, err))
		}
	}

	for _, unsub := range unsubs {
		unsub()
	}
	l.mu.Lock()
	l.unsubscribe = make(map[string]func())
	l.mu.Unlock()

	l.failedCancel(nil)

	l.refreshStatus()

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Status returns the aggregate status of the List.
func (l *List) Status() *ObservableStatus {
	return l.status
}

// refreshStatus publishes the aggregate of the resources' statuses.
//
// Reading the statuses and publishing their aggregate are two steps; when two goroutines interleave them, the
// slower one overwrites a fresher aggregate with a stale one. So a single goroutine publishes at a time, and
// recomputes for as long as changes were signaled meanwhile. A concurrent or re-entrant caller (observers run
// synchronously inside Set) only signals the change and returns, which also rules out a deadlock.
func (l *List) refreshStatus() {
	l.refreshMu.Lock()
	l.refreshPending = true
	if l.refreshing {
		l.refreshMu.Unlock()
		return
	}
	l.refreshing = true
	for l.refreshPending {
		l.refreshPending = false
		l.refreshMu.Unlock()
		l.status.Set(l.currentAggregate())
		l.refreshMu.Lock()
	}
	l.refreshing = false
	l.refreshMu.Unlock()
}

// currentAggregate reads the statuses of the resources and returns their aggregate.
func (l *List) currentAggregate() Status {
	l.mu.RLock()
	statuses := make([]Status, 0, len(l.resources))
	for _, res := range l.resources {
		statuses = append(statuses, res.Status().Get())
	}
	l.mu.RUnlock()

	return aggregateStatus(statuses)
}

// aggregateStatus returns the most severe status of the given statuses.
func aggregateStatus(statuses []Status) Status {
	if len(statuses) == 0 {
		return StatusInitializing
	}

	var unhealthy, initializing, degraded, closed bool
	allClosed := true
	for _, s := range statuses {
		switch s {
		case StatusFailed:
			return StatusFailed
		case StatusUnhealthy:
			unhealthy = true
		case StatusInitializing:
			initializing = true
		case StatusDegraded:
			degraded = true
		case StatusClosed:
			closed = true
		case StatusHealthy:
		}
		if s != StatusClosed {
			allClosed = false
		}
	}

	switch {
	case unhealthy:
		return StatusUnhealthy
	case initializing:
		return StatusInitializing
	case degraded:
		return StatusDegraded
	case allClosed:
		return StatusClosed
	case closed:
		return StatusDegraded
	default:
		return StatusHealthy
	}
}

// StatusOf returns the observable status of the named resource.
func (l *List) StatusOf(resourceName string) (*ObservableStatus, bool) {
	l.mu.RLock()
	res, ok := l.resources[resourceName]
	l.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return res.Status(), true
}

// StatusSummary returns a snapshot of every resource's current status.
func (l *List) StatusSummary() map[string]Status {
	l.mu.RLock()
	resources := make(map[string]Managed, len(l.resources))
	for name, res := range l.resources {
		resources[name] = res
	}
	l.mu.RUnlock()

	result := make(map[string]Status, len(resources))
	for name, res := range resources {
		result[name] = res.Status().Get()
	}
	return result
}

// IsHealthy reports whether every registered resource is operational.
func (l *List) IsHealthy() bool {
	l.mu.RLock()
	resources := make([]Managed, 0, len(l.resources))
	for _, res := range l.resources {
		resources = append(resources, res)
	}
	l.mu.RUnlock()

	if len(resources) == 0 {
		return true
	}
	for _, res := range resources {
		if !res.Status().Get().IsOperational() {
			return false
		}
	}
	return true
}

// List implements the Managed interface.
var _ Managed = (*List)(nil)
