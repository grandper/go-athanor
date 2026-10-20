package udp_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource/server/udp"
)

func TestNewConfigFromEnv(t *testing.T) {
	t.Run("should use the defaults when the environment is empty", func(t *testing.T) {
		t.Setenv(udp.AddressEnvVar, "")
		t.Setenv(udp.MulticastEnvVar, "")
		t.Setenv(udp.InterfaceNameEnvVar, "")
		t.Setenv(udp.BufferSizeEnvVar, "")
		t.Setenv(udp.ReceivedTimeoutEnvVar, "")

		config, err := udp.NewConfigFromEnv()

		require.NoError(t, err)
		assert.Empty(t, config.Address)
		assert.True(t, config.Multicast)
		assert.Empty(t, config.InterfaceName)
		assert.Equal(t, 1500, config.BufferSize)
		assert.Equal(t, 5*time.Second, config.ReceivedTimeout)
	})

	t.Run("should read every field from the environment", func(t *testing.T) {
		t.Setenv(udp.AddressEnvVar, "239.1.1.1:5555")
		t.Setenv(udp.MulticastEnvVar, "false")
		t.Setenv(udp.InterfaceNameEnvVar, "eth0")
		t.Setenv(udp.BufferSizeEnvVar, "9000")
		t.Setenv(udp.ReceivedTimeoutEnvVar, "20m")

		config, err := udp.NewConfigFromEnv()

		require.NoError(t, err)
		assert.Equal(t, "239.1.1.1:5555", config.Address)
		assert.False(t, config.Multicast)
		assert.Equal(t, "eth0", config.InterfaceName)
		assert.Equal(t, 9000, config.BufferSize)
		assert.Equal(t, 20*time.Minute, config.ReceivedTimeout)
	})

	t.Run("fails when the multicast flag is not a boolean", func(t *testing.T) {
		t.Setenv(udp.MulticastEnvVar, "maybe")

		config, err := udp.NewConfigFromEnv()

		require.ErrorIs(t, err, udp.ErrInvalidMulticast)
		require.ErrorContains(t, err, `failed to read UDP_MULTICAST="maybe"`)
		assert.Nil(t, config)
	})

	t.Run("fails when the buffer size is not a number", func(t *testing.T) {
		t.Setenv(udp.BufferSizeEnvVar, "big")

		config, err := udp.NewConfigFromEnv()

		require.ErrorIs(t, err, udp.ErrInvalidBufferSize)
		require.ErrorContains(t, err, `failed to read UDP_BUFFER_SIZE="big"`)
		assert.Nil(t, config)
	})

	t.Run("fails when the buffer size is not positive", func(t *testing.T) {
		for _, value := range []string{"0", "-1"} {
			t.Setenv(udp.BufferSizeEnvVar, value)

			config, err := udp.NewConfigFromEnv()

			require.ErrorIs(t, err, udp.ErrInvalidBufferSize, value)
			assert.Nil(t, config, value)
		}
	})

	t.Run("fails when the received timeout is not a duration", func(t *testing.T) {
		t.Setenv(udp.ReceivedTimeoutEnvVar, "soon")

		config, err := udp.NewConfigFromEnv()

		require.ErrorIs(t, err, udp.ErrInvalidReceivedTimeout)
		require.ErrorContains(t, err, `failed to read UDP_RECEIVED_TIMEOUT="soon"`)
		assert.Nil(t, config)
	})

	t.Run("fails when the received timeout is negative", func(t *testing.T) {
		t.Setenv(udp.ReceivedTimeoutEnvVar, "-5s")

		config, err := udp.NewConfigFromEnv()

		require.ErrorIs(t, err, udp.ErrInvalidReceivedTimeout)
		assert.Nil(t, config)
	})

	t.Run("should accept a zero received timeout, which disables the watchdog", func(t *testing.T) {
		t.Setenv(udp.ReceivedTimeoutEnvVar, "0s")

		config, err := udp.NewConfigFromEnv()

		require.NoError(t, err)
		assert.Equal(t, time.Duration(0), config.ReceivedTimeout)
	})
}
