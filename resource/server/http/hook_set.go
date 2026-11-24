package http

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

// hookSet holds the callbacks of a server, grouped by event, in registration order.
type hookSet struct {
	handlerPanicked         []func(r *http.Request, recovered any)
	errorLogged             []func(err error)
	connectionStatusChanged []func(conn net.Conn, status http.ConnState)
	shutdownStarted         []func()
}

// add appends hooks to the set. A nil Hook is ignored.
func (s *hookSet) add(hooks ...Hook) {
	for _, hook := range hooks {
		if hook != nil {
			hook(s)
		}
	}
}

// withPanicRecovery wraps next with the panic recovery when at least one OnHandlerPanicked hook exists, and
// returns next otherwise.
func (s *hookSet) withPanicRecovery(next http.Handler) http.Handler {
	if len(s.handlerPanicked) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			// An aborted handler keeps the behavior of net/http, which drops the connection without logging.
			if recovered == http.ErrAbortHandler { //nolint:errorlint // net/http compares the very value too.
				panic(recovered)
			}
			for _, callback := range s.handlerPanicked {
				callback(r, recovered)
			}
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}()
		next.ServeHTTP(w, r)
	})
}

// install wires server for the events with hooks, and leaves the fields of the events without hooks untouched.
func (s *hookSet) install(server *http.Server) {
	if len(s.errorLogged) > 0 {
		server.ErrorLog = slog.NewLogLogger(&errorLogHandler{callbacks: s.errorLogged}, slog.LevelError)
	}
	if len(s.connectionStatusChanged) > 0 {
		server.ConnState = func(conn net.Conn, status http.ConnState) {
			for _, callback := range s.connectionStatusChanged {
				callback(conn, status)
			}
		}
	}
	if len(s.shutdownStarted) > 0 {
		server.RegisterOnShutdown(func() {
			for _, callback := range s.shutdownStarted {
				callback()
			}
		})
	}
}

// errorLogHandler is the slog.Handler behind the ErrorLog of the server, handing every logged line to the
// OnErrorLogged hooks.
type errorLogHandler struct {
	callbacks []func(err error)
}

// Enabled implements slog.Handler. Every level is handed to the hooks.
func (h *errorLogHandler) Enabled(context.Context, slog.Level) bool { return true }

// Handle implements slog.Handler. It calls every OnErrorLogged hook with the logged message as an error.
func (h *errorLogHandler) Handle(_ context.Context, record slog.Record) error {
	err := errors.New(strings.TrimSuffix(record.Message, "\n"))
	for _, callback := range h.callbacks {
		callback(err)
	}
	return nil
}

// WithAttrs implements slog.Handler. The hooks receive the message only, so the attributes are dropped.
func (h *errorLogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

// WithGroup implements slog.Handler. The hooks receive the message only, so the group is dropped.
func (h *errorLogHandler) WithGroup(string) slog.Handler { return h }

// errorLogHandler implements the slog.Handler interface.
var _ slog.Handler = (*errorLogHandler)(nil)
