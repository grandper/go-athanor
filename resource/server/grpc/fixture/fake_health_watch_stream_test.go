package fixture_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/grandper/go-athanor/resource/server/grpc/fixture"
)

func TestFakeHealthWatchStream(t *testing.T) {
	ctx := context.Background()

	t.Run("should record the sent responses", func(t *testing.T) {
		stream := fixture.NewFakeHealthWatchStream(ctx)
		response := &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}

		err := stream.Send(response)

		require.NoError(t, err)
		assert.Equal(t, []*grpc_health_v1.HealthCheckResponse{response}, stream.Sent())
	})

	t.Run("should expose the context given at construction", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		stream := fixture.NewFakeHealthWatchStream(cancelCtx)

		cancel()

		assert.ErrorIs(t, stream.Context().Err(), context.Canceled)
	})

	t.Run("fails to send with the configured error", func(t *testing.T) {
		errSend := errors.New("send failed")
		stream := fixture.NewFakeHealthWatchStream(ctx).FailSendWith(errSend)

		err := stream.Send(&grpc_health_v1.HealthCheckResponse{})

		require.ErrorIs(t, err, errSend)
		assert.Empty(t, stream.Sent())
	})
}
