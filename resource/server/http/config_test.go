package http_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpserver "github.com/grandper/go-athanor/resource/server/http"
)

func TestNewConfigFromEnv(t *testing.T) {
	t.Run("should use the default bind interface and port when the environment is empty", func(t *testing.T) {
		t.Setenv(httpserver.BindInterfaceEnvVar, "")
		t.Setenv(httpserver.BindPortEnvVar, "")

		config, err := httpserver.NewConfigFromEnv()

		require.NoError(t, err)
		assert.Equal(t, "0.0.0.0", config.BindInterface)
		assert.Equal(t, 8080, config.BindPort)
	})

	t.Run("should read the bind interface and port from the environment", func(t *testing.T) {
		t.Setenv(httpserver.BindInterfaceEnvVar, "127.0.0.1")
		t.Setenv(httpserver.BindPortEnvVar, "9090")

		config, err := httpserver.NewConfigFromEnv()

		require.NoError(t, err)
		assert.Equal(t, "127.0.0.1", config.BindInterface)
		assert.Equal(t, 9090, config.BindPort)
	})

	t.Run("fails when the bind port is not a number", func(t *testing.T) {
		t.Setenv(httpserver.BindPortEnvVar, "not-a-port")

		config, err := httpserver.NewConfigFromEnv()

		require.ErrorIs(t, err, httpserver.ErrInvalidBindPort)
		require.ErrorContains(t, err, `failed to read HTTP_BIND_PORT="not-a-port"`)
		assert.Nil(t, config)
	})

	t.Run("fails when the bind port is out of range", func(t *testing.T) {
		for _, value := range []string{"0", "-1", "65536"} {
			t.Setenv(httpserver.BindPortEnvVar, value)

			config, err := httpserver.NewConfigFromEnv()

			require.ErrorIs(t, err, httpserver.ErrInvalidBindPort, value)
			assert.Nil(t, config, value)
		}
	})

	t.Run("should read the TLS certificate and key files from the environment", func(t *testing.T) {
		t.Setenv(httpserver.TLSCertFileEnvVar, "/etc/tls/cert.pem")
		t.Setenv(httpserver.TLSKeyFileEnvVar, "/etc/tls/key.pem")

		config, err := httpserver.NewConfigFromEnv()

		require.NoError(t, err)
		assert.Equal(t, "/etc/tls/cert.pem", config.TLSCertFile)
		assert.Equal(t, "/etc/tls/key.pem", config.TLSKeyFile)
	})
}

func TestConfig(t *testing.T) {
	t.Run("should format the listen address as interface:port", func(t *testing.T) {
		config := &httpserver.Config{BindInterface: "0.0.0.0", BindPort: 8080}

		assert.Equal(t, "0.0.0.0:8080", config.Address())
	})

	t.Run("should report TLS as available when both certificate and key files are set", func(t *testing.T) {
		config := &httpserver.Config{TLSCertFile: "cert.pem", TLSKeyFile: "key.pem"}

		assert.True(t, config.TLSIsAvailable())
	})

	t.Run("should report TLS as unavailable when the certificate or the key file is missing", func(t *testing.T) {
		assert.False(t, (&httpserver.Config{TLSCertFile: "cert.pem"}).TLSIsAvailable())
		assert.False(t, (&httpserver.Config{TLSKeyFile: "key.pem"}).TLSIsAvailable())
		assert.False(t, (&httpserver.Config{}).TLSIsAvailable())
	})
}
