package nats

import (
	"context"

	"github.com/nats-io/nats.go"
)

// Request carries a received NATS message and the context of its handling.
type Request struct {
	// Context is the context the message is handled within.
	Context context.Context //nolint:containedctx // The request is a short-lived argument, mirroring http.Request.

	// Msg is the received NATS message.
	Msg *nats.Msg
}
