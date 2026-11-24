// Package fixture provides reusable testing objects for the gRPC server package.
package fixture

import (
	"context"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	grpcserver "github.com/grandper/go-athanor/resource/server/grpc"
)

// FakeHandler is a configurable [grpcserver.Handler] that records its registration.
type FakeHandler struct {
	mu           sync.Mutex
	registered   bool
	registerFunc func(server *grpc.Server) error
	registerErr  error
}

// NewFakeHandler returns a FakeHandler that wires no service.
func NewFakeHandler() *FakeHandler {
	return &FakeHandler{}
}

// WithRegisterFunc makes Register delegate to fn.
func (fh *FakeHandler) WithRegisterFunc(fn func(server *grpc.Server) error) *FakeHandler {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.registerFunc = fn
	return fh
}

// FailRegisterWith makes Register fail with err.
func (fh *FakeHandler) FailRegisterWith(err error) *FakeHandler {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.registerErr = err
	return fh
}

// Register implements the grpcserver.Handler interface.
func (fh *FakeHandler) Register(server *grpc.Server) error {
	fh.mu.Lock()
	registerFunc := fh.registerFunc
	registerErr := fh.registerErr
	fh.registered = true
	fh.mu.Unlock()

	if registerErr != nil {
		return registerErr
	}
	if registerFunc != nil {
		return registerFunc(server)
	}
	return nil
}

// WasRegistered reports whether Register was called.
func (fh *FakeHandler) WasRegistered() bool {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	return fh.registered
}

// FakeHandler implements the grpcserver.Handler interface.
var _ grpcserver.Handler = (*FakeHandler)(nil)

// PanickingHandler is a [grpcserver.Handler] registering a gRPC health service whose Check and Watch panic with
// a given value.
type PanickingHandler struct {
	grpc_health_v1.UnimplementedHealthServer

	value any
}

// NewPanickingHandler returns a PanickingHandler whose health service panics with value on every request.
func NewPanickingHandler(value any) *PanickingHandler {
	return &PanickingHandler{value: value}
}

// Register implements the grpcserver.Handler interface. It registers the panicking health service on server.
func (ph *PanickingHandler) Register(server *grpc.Server) error {
	grpc_health_v1.RegisterHealthServer(server, ph)
	return nil
}

// Check implements grpc_health_v1.HealthServer. It panics with the configured value.
func (ph *PanickingHandler) Check(context.Context, *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	panic(ph.value)
}

// Watch implements grpc_health_v1.HealthServer. It panics with the configured value.
func (ph *PanickingHandler) Watch(*grpc_health_v1.HealthCheckRequest, grpc_health_v1.Health_WatchServer) error {
	panic(ph.value)
}

// PanickingHandler implements the grpcserver.Handler interface.
var _ grpcserver.Handler = (*PanickingHandler)(nil)

// PanickingHandler implements the grpc_health_v1.HealthServer interface.
var _ grpc_health_v1.HealthServer = (*PanickingHandler)(nil)
