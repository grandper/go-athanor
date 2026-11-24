package http

import (
	"net"
	"net/http"
)

// Hook is a callback of the server for one lifecycle event, built by an OnXxx constructor and given to
// Server.WithHooks.
type Hook func(set *hookSet)

// OnHandlerPanicked returns a Hook calling callback each time a handler panics. The callback receives the request
// and the value the handler panicked with; the server then answers 500 Internal Server Error and keeps serving.
// When the handler already wrote the response header, the status can no longer change. A handler aborted with
// http.ErrAbortHandler does not call the hook. Without an OnHandlerPanicked hook, the server keeps the behavior of
// net/http: it logs the panic and closes the connection. The callback runs on the goroutine of the request, so it
// must not block, and it must not call Shutdown on its own server. A nil callback is ignored.
func OnHandlerPanicked(callback func(r *http.Request, recovered any)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.handlerPanicked = append(set.handlerPanicked, callback)
		}
	}
}

// OnErrorLogged returns a Hook calling callback for each error net/http logs: TLS handshake failures, accept
// errors, and the panics of the handlers when no OnHandlerPanicked hook exists. The error carries the text
// net/http logged, its trailing newline trimmed, and is not a typed error. The callback runs on the goroutine that
// hit the error, so it must not block, and it must not call Shutdown on its own server. A nil callback is ignored.
func OnErrorLogged(callback func(err error)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.errorLogged = append(set.errorLogged, callback)
		}
	}
}

// OnConnectionStatusChanged returns a Hook calling callback on each change of status of a client connection: new,
// active, idle, hijacked, closed. The status is the http.ConnState of net/http. The callback runs on the goroutine
// of the connection, so it must not block, and it must not call Shutdown on its own server. A nil callback is
// ignored.
func OnConnectionStatusChanged(callback func(conn net.Conn, status http.ConnState)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.connectionStatusChanged = append(set.connectionStatusChanged, callback)
		}
	}
}

// OnShutdownStarted returns a Hook calling callback once, when Shutdown starts. Shutdown does not close the
// hijacked connections, such as WebSockets: this hook is where you close them. net/http runs the callback on a
// goroutine of its own and does not wait for it, so Shutdown does not either. The callback must not block, and it
// must not call Shutdown on its own server. A nil callback is ignored.
func OnShutdownStarted(callback func()) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.shutdownStarted = append(set.shutdownStarted, callback)
		}
	}
}
