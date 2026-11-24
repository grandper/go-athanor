package nats

import "github.com/nats-io/nats.go"

// Hook is a callback of the server for one lifecycle event, built by an OnXxx constructor and given to
// Server.WithHooks.
type Hook func(set *hookSet)

// OnHandlerFailed returns a Hook calling callback each time a handler, wrapped in its middlewares, returns an
// error. The callback receives the connection, the message the handler failed to process, and the error; the
// server keeps serving. The callback runs on the goroutine of the NATS client delivering the message, so it must
// not block, and it must not call Shutdown on its own server. A nil callback is ignored.
func OnHandlerFailed(callback func(nc *nats.Conn, msg *nats.Msg, err error)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.handlerFailed = append(set.handlerFailed, callback)
		}
	}
}

// OnConnectionEstablished returns a Hook calling callback once, at the end of a successful start, when the
// connection is connected. A connection that is not connected at start, for example because the client is
// reconnecting, is reported by OnConnectionRestored when it comes back instead. The callback runs on the goroutine
// starting the server, so it must not block, and it must not call Shutdown on its own server. A nil callback is
// ignored.
func OnConnectionEstablished(callback func(nc *nats.Conn)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.connectionEstablished = append(set.connectionEstablished, callback)
		}
	}
}

// OnConnectionLost returns a Hook calling callback each time the connection is lost while the server runs. The
// callback receives the error the NATS client reports, which may be nil. It runs on the callback goroutine of the
// NATS client, so it must not block, and it must not call Shutdown on its own server. A nil callback is ignored.
func OnConnectionLost(callback func(nc *nats.Conn, err error)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.connectionLost = append(set.connectionLost, callback)
		}
	}
}

// OnConnectionRestored returns a Hook calling callback each time the connection is restored while the server
// runs. The callback runs on the callback goroutine of the NATS client, so it must not block, and it must not
// call Shutdown on its own server. A nil callback is ignored.
func OnConnectionRestored(callback func(nc *nats.Conn)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.connectionRestored = append(set.connectionRestored, callback)
		}
	}
}

// OnConnectionClosed returns a Hook calling callback when the connection is closed for good while the server
// runs: the client gave up reconnecting, or someone closed it. The callback runs on the callback goroutine of the
// NATS client, so it must not block, and it must not call Shutdown on its own server. A nil callback is ignored.
func OnConnectionClosed(callback func(nc *nats.Conn)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.connectionClosed = append(set.connectionClosed, callback)
		}
	}
}

// OnSubscriptionOpened returns a Hook calling callback each time the server opens a subscription, once per
// HandleFunc registration, in registration order. The callback runs on the goroutine starting the server, while
// the server holds its lock: it must not block, and it must not call any method of the server, which would
// deadlock. A nil callback is ignored.
func OnSubscriptionOpened(callback func(nc *nats.Conn, sub *nats.Subscription)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.subscriptionOpened = append(set.subscriptionOpened, callback)
		}
	}
}

// OnSubscriptionStatusChanged returns a Hook calling callback on each status change of a subscription of the
// server. The initial nats.SubscriptionActive is not reported, only the changes that follow it, and every
// subscription ends with nats.SubscriptionClosed, including when the connection closes. The callback runs on a
// goroutine of the server, one per subscription, so it must not block, and it must not call Shutdown on its own
// server: the NATS client drops the changes a slow hook did not take yet, so a slow hook may miss transitions.
// A nil callback is ignored.
func OnSubscriptionStatusChanged(callback func(nc *nats.Conn, sub *nats.Subscription, status nats.SubStatus)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.subscriptionStatusChanged = append(set.subscriptionStatusChanged, callback)
		}
	}
}

// OnSubscriptionClosed returns a Hook calling callback once per subscription of the server when it closes:
// during Shutdown, when the connection closes, and when a failed start closes the subscriptions it already
// opened, so that every subscription reported opened is reported closed. Shutdown, and an Init that fails, wait
// for these hooks before they return. The callback runs on the callback goroutine of the NATS client, so it must
// not block, and it must not call Shutdown on its own server. A nil callback is ignored.
func OnSubscriptionClosed(callback func(nc *nats.Conn, sub *nats.Subscription)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.subscriptionClosed = append(set.subscriptionClosed, callback)
		}
	}
}
