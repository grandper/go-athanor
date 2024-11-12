package app_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/app"
)

func TestProcessFunc(t *testing.T) {
	ctx := context.Background()

	t.Run("should receive the context of the app", func(t *testing.T) {
		type contextKey struct{}
		valueCtx := context.WithValue(ctx, contextKey{}, "marker")

		var receivedValue any
		var processFunc app.ProcessFunc = func(processCtx context.Context) error {
			receivedValue = processCtx.Value(contextKey{})
			return nil
		}

		require.NoError(t, app.Run(valueCtx, app.WithProcess(processFunc)))

		assert.Equal(t, "marker", receivedValue)
	})
}
