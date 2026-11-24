package udp

// Hook is a callback of the server for one lifecycle event, built by an OnXxx constructor and given to
// Server.WithHooks.
type Hook func(set *hookSet)

// OnHandlerFailed returns a Hook calling callback each time the handler returns an error. The callback receives
// the datagram the handler failed to process and the error; the server keeps serving. The datagram holds the same
// Payload slice as the handler received, which the handler owns: a callback keeping the payload copies it whenever
// the handler may modify it. The callback runs on the read loop, before the next datagram is read, so it must not
// block, and it must not call Shutdown on its own server. A nil callback is ignored.
func OnHandlerFailed(callback func(d Datagram, err error)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.handlerFailed = append(set.handlerFailed, callback)
		}
	}
}
