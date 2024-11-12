// Package fixture provides reusable testing objects for the HTTP server package.
package fixture

import (
	"net/http"
	"sync"

	"github.com/gorilla/mux"

	httpserver "github.com/grandper/go-athanor/resource/server/http"
)

// FakeHandler is a configurable [httpserver.Handler] for tests that records its registration.
type FakeHandler struct {
	mu           sync.Mutex
	registered   bool
	registerFunc func(router *mux.Router) error
	routes       []route
	registerErr  error
}

// route is a path Register answers with a fixed status code.
type route struct {
	path string
	code int
}

// NewFakeHandler returns a FakeHandler that wires no route.
func NewFakeHandler() *FakeHandler {
	return &FakeHandler{}
}

// WithRegisterFunc makes Register delegate to fn.
func (fh *FakeHandler) WithRegisterFunc(fn func(router *mux.Router) error) *FakeHandler {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.registerFunc = fn
	return fh
}

// WithRoute makes Register add a route answering every request on path with the status code.
func (fh *FakeHandler) WithRoute(path string, code int) *FakeHandler {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.routes = append(fh.routes, route{path: path, code: code})
	return fh
}

// FailRegisterWith makes Register fail with err.
func (fh *FakeHandler) FailRegisterWith(err error) *FakeHandler {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.registerErr = err
	return fh
}

// Register implements the httpserver.Handler interface.
func (fh *FakeHandler) Register(router *mux.Router) error {
	fh.mu.Lock()
	registerFunc := fh.registerFunc
	registerErr := fh.registerErr
	routes := append([]route(nil), fh.routes...)
	fh.registered = true
	fh.mu.Unlock()

	if registerErr != nil {
		return registerErr
	}
	for _, r := range routes {
		code := r.code
		router.HandleFunc(r.path, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) })
	}

	if registerFunc != nil {
		return registerFunc(router)
	}
	return nil
}

// WasRegistered reports whether Register was called.
func (fh *FakeHandler) WasRegistered() bool {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	return fh.registered
}

// FakeHandler implements the httpserver.Handler interface.
var _ httpserver.Handler = (*FakeHandler)(nil)
