package fixture

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

const (
	servingWithin = 5 * time.Second
	servingTick   = 20 * time.Millisecond
)

// NewHealthClient returns a client of the gRPC health service served on address, over a plaintext connection
// closed when the test ends.
func NewHealthClient(t *testing.T, address string) grpc_health_v1.HealthClient {
	t.Helper()

	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return grpc_health_v1.NewHealthClient(conn)
}

// AssertEventuallyServing polls the health service until it reports service as SERVING.
// Use it to wait for a server started in the background to become reachable.
func AssertEventuallyServing(t *testing.T, client grpc_health_v1.HealthClient, service string) {
	t.Helper()

	assert.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		response, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: service})
		return err == nil && response.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING
	}, servingWithin, servingTick, "the gRPC service %q never reported SERVING", service)
}

// HealthConnection is a plaintext client connection to the gRPC health service of a server, which remembers the
// local address of the TCP connection it dials, so that a test can recognize it on the server side.
type HealthConnection struct {
	conn *grpc.ClientConn

	mu           sync.Mutex
	localAddress string
}

// NewHealthConnection returns a HealthConnection to the server on address, closed when the test ends. Like any
// gRPC client, it dials on the first request.
func NewHealthConnection(t *testing.T, address string) *HealthConnection {
	t.Helper()

	hc := &HealthConnection{}
	var dialer net.Dialer
	dial := func(ctx context.Context, target string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, "tcp", target)
		if err == nil {
			hc.mu.Lock()
			hc.localAddress = conn.LocalAddr().String()
			hc.mu.Unlock()
		}
		return conn, err
	}
	conn, err := grpc.NewClient("passthrough:///"+address,
		grpc.WithContextDialer(dial),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	hc.conn = conn
	t.Cleanup(func() { _ = conn.Close() })
	return hc
}

// Client returns a client of the health service over the connection.
func (hc *HealthConnection) Client() grpc_health_v1.HealthClient {
	return grpc_health_v1.NewHealthClient(hc.conn)
}

// LocalAddress returns the local address of the last TCP connection dialed, or an empty string before the first
// request.
func (hc *HealthConnection) LocalAddress() string {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	return hc.localAddress
}

// Close closes the connection.
func (hc *HealthConnection) Close() error {
	return hc.conn.Close()
}

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
