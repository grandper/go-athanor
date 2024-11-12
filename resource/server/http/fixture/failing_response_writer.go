package fixture

import (
	"net/http"
	"sync"
)

// FailingResponseWriter is an [http.ResponseWriter] whose writes always fail with a configured error.
type FailingResponseWriter struct {
	err    error
	header http.Header

	mu         sync.Mutex
	statusCode int
}

// NewFailingResponseWriter returns a FailingResponseWriter failing every write with err.
func NewFailingResponseWriter(err error) *FailingResponseWriter {
	return &FailingResponseWriter{
		err:    err,
		header: make(http.Header),
	}
}

// Header implements the http.ResponseWriter interface.
func (w *FailingResponseWriter) Header() http.Header {
	return w.header
}

// Write implements the http.ResponseWriter interface. It always fails with the configured error.
func (w *FailingResponseWriter) Write([]byte) (int, error) {
	return 0, w.err
}

// WriteHeader implements the http.ResponseWriter interface. It records the status code.
func (w *FailingResponseWriter) WriteHeader(statusCode int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.statusCode = statusCode
}

// StatusCode returns the status code recorded by WriteHeader.
func (w *FailingResponseWriter) StatusCode() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.statusCode
}

// FailingResponseWriter implements the http.ResponseWriter interface.
var _ http.ResponseWriter = (*FailingResponseWriter)(nil)
