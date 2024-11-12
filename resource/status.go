package resource

import "fmt"

// Status represents the health state of a resource.
type Status int

const (
	// StatusInitializing is the initial state: the resource is starting up.
	StatusInitializing Status = iota
	// StatusHealthy means the resource is fully operational.
	StatusHealthy
	// StatusDegraded means the resource is operational but experiencing issues.
	StatusDegraded
	// StatusUnhealthy means the resource is not operational but recoverable.
	StatusUnhealthy
	// StatusFailed means the resource failed unrecoverably.
	StatusFailed
	// StatusClosed means the resource was intentionally shut down.
	StatusClosed
)

// String returns the upper-case name of the status.
func (s Status) String() string {
	switch s {
	case StatusInitializing:
		return "INITIALIZING"
	case StatusHealthy:
		return "HEALTHY"
	case StatusDegraded:
		return "DEGRADED"
	case StatusUnhealthy:
		return "UNHEALTHY"
	case StatusFailed:
		return "FAILED"
	case StatusClosed:
		return "CLOSED"
	default:
		return "UNKNOWN"
	}
}

// IsOperational reports whether the resource is healthy or degraded.
func (s Status) IsOperational() bool {
	return s == StatusHealthy || s == StatusDegraded
}

// IsTerminal reports whether the resource has stopped for good.
func (s Status) IsTerminal() bool {
	return s == StatusClosed || s == StatusFailed
}

// IsFailed reports whether the resource failed unrecoverably.
func (s Status) IsFailed() bool {
	return s == StatusFailed
}

// Status implements the fmt.Stringer interface.
var _ fmt.Stringer = Status(0)
