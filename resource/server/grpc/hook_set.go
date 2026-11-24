package grpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

// hookSet holds the callbacks of a server, grouped by event, in registration order.
type hookSet struct {
	handlerFailed         []func(ctx context.Context, method string, err error)
	handlerPanicked       []func(ctx context.Context, method string, recovered any)
	connectionEstablished []func(ctx context.Context, info *stats.ConnTagInfo)
	connectionClosed      []func(ctx context.Context, info *stats.ConnTagInfo)
	requestStatsReported  []func(ctx context.Context, requestStats stats.RPCStats)
}

// add appends hooks to the set. A nil Hook is ignored.
func (s *hookSet) add(hooks ...Hook) {
	for _, hook := range hooks {
		if hook != nil {
			hook(s)
		}
	}
}

// serverOptions returns the interceptors when at least one OnHandlerFailed or OnHandlerPanicked hook exists, the
// failure interceptor outermost so that it also sees the codes.Internal error of a recovered panic, and the stats
// handler when at least one OnConnectionEstablished, OnConnectionClosed or OnRequestStatsReported hook exists.
func (s *hookSet) serverOptions() []grpc.ServerOption {
	var unary []grpc.UnaryServerInterceptor
	var stream []grpc.StreamServerInterceptor
	if len(s.handlerFailed) > 0 {
		unary = append(unary, s.unaryFailure)
		stream = append(stream, s.streamFailure)
	}
	if len(s.handlerPanicked) > 0 {
		unary = append(unary, s.unaryRecovery)
		stream = append(stream, s.streamRecovery)
	}
	var options []grpc.ServerOption
	if len(unary) > 0 {
		options = append(options, grpc.ChainUnaryInterceptor(unary...), grpc.ChainStreamInterceptor(stream...))
	}
	if len(s.connectionEstablished) > 0 || len(s.connectionClosed) > 0 || len(s.requestStatsReported) > 0 {
		options = append(options, grpc.StatsHandler(&statsHandler{hooks: s}))
	}
	return options
}

// unaryFailure reports the error of a unary handler to the OnHandlerFailed hooks.
func (s *hookSet) unaryFailure(
	ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (any, error) {
	resp, err := handler(ctx, req)
	if err != nil {
		s.notifyHandlerFailed(ctx, info.FullMethod, err)
	}
	return resp, err
}

// streamFailure reports the error of a stream handler to the OnHandlerFailed hooks.
func (s *hookSet) streamFailure(
	srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler,
) error {
	err := handler(srv, stream)
	if err != nil {
		s.notifyHandlerFailed(stream.Context(), info.FullMethod, err)
	}
	return err
}

// unaryRecovery turns the panic of a unary handler into a codes.Internal error, reported to the OnHandlerPanicked
// hooks.
func (s *hookSet) unaryRecovery(
	ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (any, error) {
	var resp any
	err := s.recovering(ctx, info.FullMethod, func() error {
		var errHandle error
		resp, errHandle = handler(ctx, req)
		return errHandle
	})
	return resp, err
}

// streamRecovery turns the panic of a stream handler into a codes.Internal error, reported to the
// OnHandlerPanicked hooks.
func (s *hookSet) streamRecovery(
	srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler,
) error {
	return s.recovering(stream.Context(), info.FullMethod, func() error { return handler(srv, stream) })
}

// recovering returns the error of call, or reports its panic to the OnHandlerPanicked hooks and returns the
// codes.Internal error the request fails with.
func (s *hookSet) recovering(ctx context.Context, method string, call func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			for _, callback := range s.handlerPanicked {
				callback(ctx, method, recovered)
			}
			err = status.Error(codes.Internal, "internal error")
		}
	}()
	return call()
}

// notifyHandlerFailed calls every OnHandlerFailed hook, in registration order.
func (s *hookSet) notifyHandlerFailed(ctx context.Context, method string, err error) {
	for _, callback := range s.handlerFailed {
		callback(ctx, method, err)
	}
}

// connTagInfoKey is the context key under which statsHandler stores the *stats.ConnTagInfo of a connection.
type connTagInfoKey struct{}

// statsHandler is the stats.Handler of the server, handing the connection and request statistics to the hooks.
type statsHandler struct {
	hooks *hookSet
}

// TagConn implements stats.Handler. It stores info in the context of the connection, for HandleConn.
func (h *statsHandler) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	return context.WithValue(ctx, connTagInfoKey{}, info)
}

// HandleConn implements stats.Handler. It calls the OnConnectionEstablished hooks when a connection begins, and
// the OnConnectionClosed hooks when it ends.
func (h *statsHandler) HandleConn(ctx context.Context, connStats stats.ConnStats) {
	info, _ := ctx.Value(connTagInfoKey{}).(*stats.ConnTagInfo)
	switch connStats.(type) {
	case *stats.ConnBegin:
		for _, callback := range h.hooks.connectionEstablished {
			callback(ctx, info)
		}
	case *stats.ConnEnd:
		for _, callback := range h.hooks.connectionClosed {
			callback(ctx, info)
		}
	}
}

// TagRPC implements stats.Handler. It returns ctx unchanged.
func (h *statsHandler) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	return ctx
}

// HandleRPC implements stats.Handler. It calls the OnRequestStatsReported hooks.
func (h *statsHandler) HandleRPC(ctx context.Context, requestStats stats.RPCStats) {
	for _, callback := range h.hooks.requestStatsReported {
		callback(ctx, requestStats)
	}
}

// statsHandler implements the stats.Handler interface.
var _ stats.Handler = (*statsHandler)(nil)
