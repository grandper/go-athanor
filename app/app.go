package app

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/grandper/go-athanor/process"
	"github.com/grandper/go-athanor/resource"
)

// App is the application assembled by [Run] from its hooks, resources, and processes.
type App struct {
	hooksBefore []ProcessFunc
	hooksAfter  []ProcessFunc
	processes   []ProcessFunc
	resources   *resource.List
}

// Run configures an App with the options and runs its hooks, resources, and processes until they stop.
func Run(ctx context.Context, options ...Option) error {
	app := &App{}
	for _, option := range options {
		if err := option(app); err != nil {
			return fmt.Errorf("failed to configure the app: %w", err)
		}
	}
	return app.run(ctx)
}

// run executes the before hooks, the resources and processes, then the after hooks.
func (a *App) run(ctx context.Context) error {
	for _, hook := range a.hooksBefore {
		if err := hook(ctx); err != nil {
			return fmt.Errorf("before hook failed: %w", err)
		}
	}

	processErr := a.runResourcesAndProcesses(ctx)

	return errors.Join(processErr, a.runHooksAfter(ctx))
}

// runHooksAfter runs every after hook, even when one fails, and joins their failures.
// The hooks clean up, so they keep the values of ctx but not its cancellation, which is often what stopped the app.
func (a *App) runHooksAfter(ctx context.Context) error {
	cleanupCtx, cancel := resource.CleanupContext(ctx)
	defer cancel()

	var errs []error
	for _, hook := range a.hooksAfter {
		if err := hook(cleanupCtx); err != nil {
			errs = append(errs, fmt.Errorf("after hook failed: %w", err))
		}
	}
	return errors.Join(errs...)
}

// runResourcesAndProcesses initializes the resources, runs the processes, then shuts the resources down.
func (a *App) runResourcesAndProcesses(ctx context.Context) error {
	if !a.hasResources() {
		return ignoreSignal(a.runParticipants(ctx))
	}

	if err := a.resources.Init(ctx); err != nil {
		// Returned as a process error so the after hooks still run.
		return fmt.Errorf("failed to initialize the resources: %w", err)
	}

	runErr := ignoreSignal(a.runParticipants(ctx))

	return errors.Join(runErr, a.shutdownResources(ctx))
}

// ignoreSignal drops the error of the POSIX signal listener: a signal is a graceful shutdown, not an error.
func ignoreSignal(err error) error {
	if errors.Is(err, process.ErrPOSIXSignalReceived) {
		return nil
	}
	return err
}

// runParticipants runs the resources and processes concurrently until the first one stops.
func (a *App) runParticipants(ctx context.Context) error {
	g, groupCtx := errgroup.WithContext(ctx)

	if a.hasResources() {
		g.Go(func() error { return gracefulManage(groupCtx, a.resources) })
	}
	for _, p := range a.processes {
		p := p // shared loop variable before go 1.22
		g.Go(func() error { return p(groupCtx) })
	}

	return g.Wait()
}

// shutdownResources shuts the resources down. It is a cleanup, so it keeps the values of ctx but not its
// cancellation, which is often what stopped the app.
func (a *App) shutdownResources(ctx context.Context) error {
	shutdownCtx, cancel := resource.CleanupContext(ctx)
	defer cancel()

	return a.resources.Shutdown(shutdownCtx)
}

// hasResources reports whether a resource was registered.
func (a *App) hasResources() bool {
	return a.resources != nil && len(a.resources.StatusSummary()) > 0
}

// gracefulManage waits for a resource failure, treating a canceled context as a graceful stop.
func gracefulManage(ctx context.Context, resources *resource.List) error {
	err := resources.Manage(ctx)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}
