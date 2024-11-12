package nats

import "github.com/nats-io/nats.go"

// HandlerFunc processes a received message and can reply through publish.
type HandlerFunc func(r *Request, publish PublishFunc) error

// PublishFunc publishes a message, typically to reply to a request.
type PublishFunc func(msg *nats.Msg) error
