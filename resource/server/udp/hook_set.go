package udp

// hookSet holds the callbacks of a server, grouped by event, in registration order.
type hookSet struct {
	handlerFailed []func(d Datagram, err error)
}

// add appends hooks to the set. A nil Hook is ignored.
func (s *hookSet) add(hooks ...Hook) {
	for _, hook := range hooks {
		if hook != nil {
			hook(s)
		}
	}
}

// notifyHandlerFailed calls every OnHandlerFailed hook, in registration order.
func (s *hookSet) notifyHandlerFailed(d Datagram, err error) {
	for _, callback := range s.handlerFailed {
		callback(d, err)
	}
}
