package grpc

import (
	"context"

	"google.golang.org/grpc/stats"
)

// Hook is a callback of the server for one lifecycle event, built by an OnXxx constructor and given to
// Server.WithHooks.
type Hook func(set *hookSet)

// OnHandlerFailed returns a Hook calling callback for each unary or stream request whose handler returns an error,
// including the codes.Internal error of a panic an OnHandlerPanicked hook recovered. The callback receives the
// context of the request, the full method name, and the error, whose gRPC code status.Code gives. The callback
// runs on the goroutine of the request, so it must not block, and it must not call Shutdown on its own server.
// A nil callback is ignored.
func OnHandlerFailed(callback func(ctx context.Context, method string, err error)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.handlerFailed = append(set.handlerFailed, callback)
		}
	}
}

// OnHandlerPanicked returns a Hook calling callback each time a unary or stream handler panics. The callback
// receives the context of the request, the full method name, and the value the handler panicked with; the request
// then fails with codes.Internal and the server keeps serving. Without an OnHandlerPanicked hook, the server keeps
// the behavior of gRPC, and a panic crashes the process. The callback runs on the goroutine of the request, so it
// must not block, and it must not call Shutdown on its own server. A nil callback is ignored.
func OnHandlerPanicked(callback func(ctx context.Context, method string, recovered any)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.handlerPanicked = append(set.handlerPanicked, callback)
		}
	}
}

// OnConnectionEstablished returns a Hook calling callback each time a client connection is established. The
// callback receives the context of the connection and its *stats.ConnTagInfo, holding the remote and local
// addresses. The callback runs on the goroutine of the transport, so it must not block, and it must not call
// Shutdown on its own server. A nil callback is ignored.
func OnConnectionEstablished(callback func(ctx context.Context, info *stats.ConnTagInfo)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.connectionEstablished = append(set.connectionEstablished, callback)
		}
	}
}

// OnConnectionClosed returns a Hook calling callback each time a client connection closes, including the
// connections Shutdown closes. The callback receives the context of the connection and its *stats.ConnTagInfo.
// gRPC does not guarantee that a connection is reported closed before its server stops, so Shutdown does not wait
// for this hook. The callback runs on the goroutine of the transport, so it must not block, and it must not call
// Shutdown on its own server. A nil callback is ignored.
func OnConnectionClosed(callback func(ctx context.Context, info *stats.ConnTagInfo)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.connectionClosed = append(set.connectionClosed, callback)
		}
	}
}

// OnRequestStatsReported returns a Hook calling callback for each statistics event of each request: begin,
// headers, payloads, trailers, end. The callback receives the context of the request and the stats.RPCStats event,
// such as *stats.Begin or *stats.End. It fires very often, so it must be cheap. The callback runs on the goroutine
// of the request, so it must not block, and it must not call Shutdown on its own server. A nil callback is
// ignored.
func OnRequestStatsReported(callback func(ctx context.Context, requestStats stats.RPCStats)) Hook {
	return func(set *hookSet) {
		if callback != nil {
			set.requestStatsReported = append(set.requestStatsReported, callback)
		}
	}
}
