package grpc_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	grpcserver "github.com/grandper/go-athanor/resource/server/grpc"
)

func TestNewConfigFromEnv(t *testing.T) {
	t.Run("should use the default bind interface and port when the environment is empty", func(t *testing.T) {
		t.Setenv(grpcserver.BindInterfaceEnvVar, "")
		t.Setenv(grpcserver.BindPortEnvVar, "")

		config, err := grpcserver.NewConfigFromEnv()

		require.NoError(t, err)
		assert.Equal(t, "0.0.0.0", config.BindInterface)
		assert.Equal(t, 50051, config.BindPort)
	})

	t.Run("should read the bind interface and port from the environment", func(t *testing.T) {
		t.Setenv(grpcserver.BindInterfaceEnvVar, "127.0.0.1")
		t.Setenv(grpcserver.BindPortEnvVar, "7000")

		config, err := grpcserver.NewConfigFromEnv()

		require.NoError(t, err)
		assert.Equal(t, "127.0.0.1", config.BindInterface)
		assert.Equal(t, 7000, config.BindPort)
	})

	t.Run("fails when the bind port is not a number", func(t *testing.T) {
		t.Setenv(grpcserver.BindPortEnvVar, "not-a-port")

		config, err := grpcserver.NewConfigFromEnv()

		require.ErrorIs(t, err, grpcserver.ErrInvalidBindPort)
		require.ErrorContains(t, err, `failed to read GRPC_BIND_PORT="not-a-port"`)
		assert.Nil(t, config)
	})

	t.Run("fails when the bind port is out of range", func(t *testing.T) {
		for _, value := range []string{"0", "-1", "65536"} {
			t.Setenv(grpcserver.BindPortEnvVar, value)

			config, err := grpcserver.NewConfigFromEnv()

			require.ErrorIs(t, err, grpcserver.ErrInvalidBindPort, value)
			assert.Nil(t, config, value)
		}
	})
}

func TestConfig(t *testing.T) {
	t.Run("should format the listen address as interface:port", func(t *testing.T) {
		config := &grpcserver.Config{BindInterface: "0.0.0.0", BindPort: 50051}

		assert.Equal(t, "0.0.0.0:50051", config.Address())
	})
}
