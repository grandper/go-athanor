package nats

// MiddlewareFunc wraps a HandlerFunc.
type MiddlewareFunc func(next HandlerFunc) HandlerFunc
