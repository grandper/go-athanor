package fixture

import (
	"sync"

	natsserver "github.com/grandper/go-athanor/resource/server/nats"
)

// FakeHandler is a configurable [natsserver.Handler] that records its registration.
type FakeHandler struct {
	mu           sync.Mutex
	registered   bool
	registerFunc func(server *natsserver.Server) error
	handleFuncs  []handleFunc
	registerErr  error
}

// handleFunc is a handler function Register subscribes on the server.
type handleFunc struct {
	subject string
	queue   string
	handler natsserver.HandlerFunc
}

// NewFakeHandler returns a FakeHandler that wires no subscription.
func NewFakeHandler() *FakeHandler {
	return &FakeHandler{}
}

// WithRegisterFunc makes Register delegate to fn.
func (fh *FakeHandler) WithRegisterFunc(fn func(server *natsserver.Server) error) *FakeHandler {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.registerFunc = fn
	return fh
}

// WithHandleFunc makes Register subscribe handler on subject, optionally in a queue group.
func (fh *FakeHandler) WithHandleFunc(subject string, queue string, handler natsserver.HandlerFunc) *FakeHandler {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.handleFuncs = append(fh.handleFuncs, handleFunc{subject: subject, queue: queue, handler: handler})
	return fh
}

// FailRegisterWith makes Register fail with err.
func (fh *FakeHandler) FailRegisterWith(err error) *FakeHandler {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.registerErr = err
	return fh
}

// Register implements the natsserver.Handler interface.
func (fh *FakeHandler) Register(server *natsserver.Server) error {
	fh.mu.Lock()
	registerFunc := fh.registerFunc
	registerErr := fh.registerErr
	handleFuncs := append([]handleFunc(nil), fh.handleFuncs...)
	fh.registered = true
	fh.mu.Unlock()

	if registerErr != nil {
		return registerErr
	}
	for _, hf := range handleFuncs {
		if err := server.HandleFunc(hf.subject, hf.queue, hf.handler); err != nil {
			return err
		}
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

// FakeHandler implements the natsserver.Handler interface.
var _ natsserver.Handler = (*FakeHandler)(nil)
