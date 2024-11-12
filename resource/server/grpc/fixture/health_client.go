package fixture

import (
	"context"
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
