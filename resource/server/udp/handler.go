package udp

import (
	"context"
	"net/netip"
)

// Datagram is a UDP payload received by a Server.
type Datagram struct {
	// Payload holds the bytes of the datagram, truncated to the buffer size of the server. The handler owns it.
	Payload []byte

	// Source is the address the datagram was sent from.
	Source netip.AddrPort
}

// Handler processes the datagrams received by a Server, one at a time, on the goroutine reading the socket.
type Handler interface {
	// Handle processes one datagram. An error is reported to the callback set with Server.OnError and does not
	// stop the server. The context is canceled when the server shuts down.
	Handle(ctx context.Context, datagram Datagram) error
}

// HandlerFunc adapts an ordinary function to a Handler.
type HandlerFunc func(ctx context.Context, datagram Datagram) error

// Handle implements the Handler interface. It calls the function.
func (f HandlerFunc) Handle(ctx context.Context, datagram Datagram) error {
	return f(ctx, datagram)
}

// HandlerFunc implements the Handler interface.
var _ Handler = (HandlerFunc)(nil)
