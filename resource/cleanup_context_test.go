package resource_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource"
)

func TestCleanupContext(t *testing.T) {
	type contextKey struct{}

	t.Run("should keep the values of the context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), contextKey{}, "trace-42")

		cleanupCtx, cancel := resource.CleanupContext(ctx)
		defer cancel()

		assert.Equal(t, "trace-42", cleanupCtx.Value(contextKey{}))
	})

	t.Run("should not be canceled with the context", func(t *testing.T) {
		ctx, cancelParent := context.WithCancel(context.Background())
		cancelParent()

		cleanupCtx, cancel := resource.CleanupContext(ctx)
		defer cancel()

		assert.NoError(t, cleanupCtx.Err())
	})

	t.Run("should be bounded by CleanupTimeout", func(t *testing.T) {
		cleanupCtx, cancel := resource.CleanupContext(context.Background())
		defer cancel()

		deadline, ok := cleanupCtx.Deadline()

		require.True(t, ok, "the cleanup must be bounded")
		assert.WithinDuration(t, time.Now().Add(resource.CleanupTimeout), deadline, time.Second)
	})

	t.Run("should be canceled by its cancel function", func(t *testing.T) {
		cleanupCtx, cancel := resource.CleanupContext(context.Background())

		cancel()

		assert.ErrorIs(t, cleanupCtx.Err(), context.Canceled)
	})
}
