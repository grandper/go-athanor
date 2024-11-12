package resource

import "context"

// Managed is a resource whose lifecycle and status are managed.
type Managed interface {
	// Name returns a unique identifier for this resource.
	Name() string

	// Status returns the resource's observable status, available before Init.
	Status() *ObservableStatus

	// Init initializes the resource and starts its background work.
	Init(ctx context.Context) error

	// Shutdown gracefully shuts down the resource, setting its status to StatusClosed or StatusFailed.
	Shutdown(ctx context.Context) error
}
