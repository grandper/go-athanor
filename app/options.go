package app

import (
	"fmt"

	"github.com/grandper/go-athanor/resource"
)

// Option configures the [App] assembled by [Run].
type Option func(*App) error

// WithHookBefore adds a hook run before the processes start.
func WithHookBefore(hook ProcessFunc) Option {
	return WithHooksBefore(hook)
}

// WithHooksBefore adds hooks run before the processes start.
func WithHooksBefore(hooks ...ProcessFunc) Option {
	return func(a *App) error {
		a.hooksBefore = append(a.hooksBefore, hooks...)
		return nil
	}
}

// WithHookAfter adds a hook run after the processes have stopped.
func WithHookAfter(hook ProcessFunc) Option {
	return WithHooksAfter(hook)
}

// WithHooksAfter adds hooks run after the processes have stopped.
func WithHooksAfter(hooks ...ProcessFunc) Option {
	return func(a *App) error {
		a.hooksAfter = append(a.hooksAfter, hooks...)
		return nil
	}
}

// WithResource registers a managed resource driven by the app lifecycle.
func WithResource(r resource.Managed) Option {
	return WithResources(r)
}

// WithResources registers several managed resources, initialized in order and shut down in reverse.
func WithResources(resources ...resource.Managed) Option {
	return func(a *App) error {
		if a.resources == nil {
			a.resources = resource.NewList()
		}
		for _, r := range resources {
			if err := a.resources.Register(r); err != nil {
				return fmt.Errorf("failed to register a resource: %w", err)
			}
		}
		return nil
	}
}

// WithProcess adds a custom process run concurrently with the resources.
func WithProcess(process ProcessFunc) Option {
	return WithProcesses(process)
}

// WithProcesses adds custom processes run concurrently with the resources.
func WithProcesses(processes ...ProcessFunc) Option {
	return func(a *App) error {
		a.processes = append(a.processes, processes...)
		return nil
	}
}
