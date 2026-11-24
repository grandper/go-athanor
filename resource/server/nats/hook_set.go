package nats

import "github.com/nats-io/nats.go"

// hookSet holds the callbacks of a server, grouped by event, in registration order.
type hookSet struct {
	connectionEstablished     []func(nc *nats.Conn)
	connectionLost            []func(nc *nats.Conn, err error)
	connectionRestored        []func(nc *nats.Conn)
	connectionClosed          []func(nc *nats.Conn)
	subscriptionOpened        []func(nc *nats.Conn, sub *nats.Subscription)
	subscriptionStatusChanged []func(nc *nats.Conn, sub *nats.Subscription, status nats.SubStatus)
	subscriptionClosed        []func(nc *nats.Conn, sub *nats.Subscription)
	handlerFailed             []func(nc *nats.Conn, msg *nats.Msg, err error)
}

// add appends hooks to the set. A nil Hook is ignored.
func (s *hookSet) add(hooks ...Hook) {
	for _, hook := range hooks {
		if hook != nil {
			hook(s)
		}
	}
}

// notifyConnectionEstablished calls every OnConnectionEstablished hook, in registration order.
func (s *hookSet) notifyConnectionEstablished(nc *nats.Conn) {
	for _, callback := range s.connectionEstablished {
		callback(nc)
	}
}

// disconnectErrHandler returns the callback running every OnConnectionLost hook, or nil when there is none.
func (s *hookSet) disconnectErrHandler() nats.ConnErrHandler {
	if len(s.connectionLost) == 0 {
		return nil
	}
	return func(nc *nats.Conn, err error) {
		for _, callback := range s.connectionLost {
			callback(nc, err)
		}
	}
}

// reconnectHandler returns the callback running every OnConnectionRestored hook, or nil when there is none.
func (s *hookSet) reconnectHandler() nats.ConnHandler {
	return connHandler(s.connectionRestored)
}

// closedHandler returns the callback running every OnConnectionClosed hook, or nil when there is none.
func (s *hookSet) closedHandler() nats.ConnHandler {
	return connHandler(s.connectionClosed)
}

// notifySubscriptionOpened calls every OnSubscriptionOpened hook, in registration order.
func (s *hookSet) notifySubscriptionOpened(nc *nats.Conn, sub *nats.Subscription) {
	for _, callback := range s.subscriptionOpened {
		callback(nc, sub)
	}
}

// subscriptionClosedHandler returns the closed handler of sub running every OnSubscriptionClosed hook, or nil
// when there is none.
func (s *hookSet) subscriptionClosedHandler(nc *nats.Conn, sub *nats.Subscription) func(subject string) {
	if len(s.subscriptionClosed) == 0 {
		return nil
	}
	return func(string) {
		for _, callback := range s.subscriptionClosed {
			callback(nc, sub)
		}
	}
}

// watchesSubscriptionStatus reports whether at least one OnSubscriptionStatusChanged hook exists.
func (s *hookSet) watchesSubscriptionStatus() bool {
	return len(s.subscriptionStatusChanged) > 0
}

// notifySubscriptionStatusChanged calls every OnSubscriptionStatusChanged hook, in registration order.
func (s *hookSet) notifySubscriptionStatusChanged(nc *nats.Conn, sub *nats.Subscription, status nats.SubStatus) {
	for _, callback := range s.subscriptionStatusChanged {
		callback(nc, sub, status)
	}
}

// notifyHandlerFailed calls every OnHandlerFailed hook, in registration order.
func (s *hookSet) notifyHandlerFailed(nc *nats.Conn, msg *nats.Msg, err error) {
	for _, callback := range s.handlerFailed {
		callback(nc, msg, err)
	}
}

// connHandler returns a nats.ConnHandler calling callbacks in order, or nil when there is none.
func connHandler(callbacks []func(nc *nats.Conn)) nats.ConnHandler {
	if len(callbacks) == 0 {
		return nil
	}
	return func(nc *nats.Conn) {
		for _, callback := range callbacks {
			callback(nc)
		}
	}
}
