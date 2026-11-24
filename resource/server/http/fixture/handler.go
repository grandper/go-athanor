// Package fixture provides reusable testing objects for the HTTP server package.
package fixture

import (
	"net"
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

// PanickingHandler is an [httpserver.Handler] registering a route whose handler panics with a given value.
type PanickingHandler struct {
	path  string
	value any
}

// NewPanickingHandler returns a PanickingHandler whose route on path panics with value on every request.
func NewPanickingHandler(path string, value any) *PanickingHandler {
	return &PanickingHandler{path: path, value: value}
}

// Register implements the httpserver.Handler interface.
func (ph *PanickingHandler) Register(router *mux.Router) error {
	value := ph.value
	router.HandleFunc(ph.path, func(http.ResponseWriter, *http.Request) { panic(value) })
	return nil
}

// PanickingHandler implements the httpserver.Handler interface.
var _ httpserver.Handler = (*PanickingHandler)(nil)

// switchingProtocols is the response a HijackingHandler writes on the connections it takes over.
const switchingProtocols = "HTTP/1.1 101 Switching Protocols\r\n\r\n"

// HijackingHandler is an [httpserver.Handler] registering a route that takes the connection over from net/http,
// as a WebSocket upgrade does, and keeps it open until CloseAll. net/http does not close those connections when
// the server shuts down.
type HijackingHandler struct {
	path string

	mu    sync.Mutex
	conns []net.Conn
}

// NewHijackingHandler returns a HijackingHandler whose route on path hijacks every connection it serves.
func NewHijackingHandler(path string) *HijackingHandler {
	return &HijackingHandler{path: path}
}

// Register implements the httpserver.Handler interface.
func (hh *HijackingHandler) Register(router *mux.Router) error {
	router.HandleFunc(hh.path, func(w http.ResponseWriter, _ *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijacking is not supported", http.StatusInternalServerError)
			return
		}
		conn, buffer, err := hijacker.Hijack()
		if err != nil {
			return
		}
		if _, errWrite := buffer.WriteString(switchingProtocols); errWrite != nil || buffer.Flush() != nil {
			_ = conn.Close()
			return
		}
		hh.mu.Lock()
		defer hh.mu.Unlock()
		hh.conns = append(hh.conns, conn)
	})
	return nil
}

// Hijacked returns the number of connections taken over so far.
func (hh *HijackingHandler) Hijacked() int {
	hh.mu.Lock()
	defer hh.mu.Unlock()
	return len(hh.conns)
}

// CloseAll closes every connection taken over so far.
func (hh *HijackingHandler) CloseAll() {
	hh.mu.Lock()
	defer hh.mu.Unlock()
	for _, conn := range hh.conns {
		_ = conn.Close()
	}
	hh.conns = nil
}

// HijackingHandler implements the httpserver.Handler interface.
var _ httpserver.Handler = (*HijackingHandler)(nil)
