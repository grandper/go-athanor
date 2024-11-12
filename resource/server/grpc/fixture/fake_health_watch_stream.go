package fixture

import (
	"context"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// FakeHealthWatchStream is an in-memory server stream of the gRPC health Watch call that records
// the sent responses.
type FakeHealthWatchStream struct {
	// ServerStream is never set: only the methods a health server uses are implemented.
	grpc.ServerStream

	ctx context.Context //nolint:containedctx // The stream carries the context of the call, like a gRPC stream.

	mu      sync.Mutex
	sent    []*grpc_health_v1.HealthCheckResponse
	sendErr error
}

// NewFakeHealthWatchStream returns a FakeHealthWatchStream whose call runs within ctx.
func NewFakeHealthWatchStream(ctx context.Context) *FakeHealthWatchStream {
	return &FakeHealthWatchStream{ctx: ctx}
}

// FailSendWith makes every subsequent Send fail with err.
func (s *FakeHealthWatchStream) FailSendWith(err error) *FakeHealthWatchStream {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sendErr = err
	return s
}

// Send implements grpc_health_v1.Health_WatchServer. It records response.
func (s *FakeHealthWatchStream) Send(response *grpc_health_v1.HealthCheckResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sendErr != nil {
		return s.sendErr
	}
	s.sent = append(s.sent, response)
	return nil
}

// Context implements grpc_health_v1.Health_WatchServer.
func (s *FakeHealthWatchStream) Context() context.Context {
	return s.ctx
}

// Sent returns a snapshot of the responses sent so far.
func (s *FakeHealthWatchStream) Sent() []*grpc_health_v1.HealthCheckResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*grpc_health_v1.HealthCheckResponse(nil), s.sent...)
}

// FakeHealthWatchStream implements the grpc_health_v1.Health_WatchServer interface.
var _ grpc_health_v1.Health_WatchServer = (*FakeHealthWatchStream)(nil)
