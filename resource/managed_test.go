package resource_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource"
	"github.com/grandper/go-athanor/resource/fixture"
)

func TestManaged(t *testing.T) {
	ctx := context.Background()

	t.Run("should drive its status through its lifecycle", func(t *testing.T) {
		var r resource.Managed = fixture.NewHealthyFakeManaged("db")

		assert.Equal(t, resource.StatusInitializing, r.Status().Get())

		require.NoError(t, r.Init(ctx))
		assert.Equal(t, resource.StatusHealthy, r.Status().Get())

		require.NoError(t, r.Shutdown(ctx))
		assert.Equal(t, resource.StatusClosed, r.Status().Get())
	})

	t.Run("should report StatusFailed when initialization fails", func(t *testing.T) {
		errBoom := errors.New("boom")
		var r resource.Managed = fixture.NewFakeManaged("db").FailInitWith(errBoom)

		require.ErrorIs(t, r.Init(ctx), errBoom)
		assert.Equal(t, resource.StatusFailed, r.Status().Get())
	})
}
