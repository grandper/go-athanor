// Package fixture provides reusable testing objects for the gRPC server package.
package fixture

import (
	"sync"

	"google.golang.org/grpc"

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
