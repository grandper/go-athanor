package fixture_test

import (
	"context"
	"fmt"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpserver "github.com/grandper/go-athanor/resource/server/http"
	"github.com/grandper/go-athanor/resource/server/http/fixture"
)

func TestInitOnFreePort(t *testing.T) {
	ctx := context.Background()

	t.Run("should initialize a server bound to a free port", func(t *testing.T) {
		config := &httpserver.Config{BindInterface: "127.0.0.1"}
		server := httpserver.NewServer(config)

		err := fixture.InitOnFreePort(ctx, t, config, server.Init)

		require.NoError(t, err)
		defer func() { require.NoError(t, server.Shutdown(ctx)) }()
		assert.Positive(t, config.BindPort)
	})

	t.Run("should initialize again on another port when the address is already in use", func(t *testing.T) {
		config := &httpserver.Config{BindInterface: "127.0.0.1"}
		var ports []int

		err := fixture.InitOnFreePort(ctx, t, config, func(context.Context) error {
			ports = append(ports, config.BindPort)
			if len(ports) == 1 {
				return fmt.Errorf("failed to listen: %w", syscall.EADDRINUSE)
			}
			return nil
		})

		require.NoError(t, err)
		assert.Len(t, ports, 2)
	})

	t.Run("fails when the initialization fails", func(t *testing.T) {
		config := &httpserver.Config{BindInterface: "127.0.0.1"}

		err := fixture.InitOnFreePort(ctx, t, config, func(context.Context) error { return assert.AnError })

		assert.ErrorIs(t, err, assert.AnError)
	})
}
