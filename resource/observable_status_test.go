package resource_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource"
)

func TestObservableStatus(t *testing.T) {
	t.Run("should return the initial value", func(t *testing.T) {
		obs := resource.NewObservableStatus(resource.StatusInitializing)
		require.Equal(t, resource.StatusInitializing, obs.Get())
	})

	t.Run("should notify observers of every change", func(t *testing.T) {
		obs := resource.NewObservableStatus(resource.StatusInitializing)

		var got []resource.Status
		obs.Observe(resource.StatusObserverFunc(func(s resource.Status) {
			got = append(got, s)
		}))

		obs.Set(resource.StatusHealthy)
		obs.Set(resource.StatusDegraded)

		assert.Equal(t, resource.StatusDegraded, obs.Get())
		assert.Equal(t, []resource.Status{resource.StatusHealthy, resource.StatusDegraded}, got)
	})

	t.Run("should not notify when the value does not change", func(t *testing.T) {
		obs := resource.NewObservableStatus(resource.StatusHealthy)

		notified := 0
		obs.Observe(resource.StatusObserverFunc(func(resource.Status) { notified++ }))

		obs.Set(resource.StatusHealthy)

		assert.Zero(t, notified, "setting the same value must not notify")
	})

	t.Run("should stop notifying after unsubscribe", func(t *testing.T) {
		obs := resource.NewObservableStatus(resource.StatusInitializing)

		notified := 0
		unsubscribe := obs.Observe(resource.StatusObserverFunc(func(resource.Status) { notified++ }))

		obs.Set(resource.StatusHealthy)
		unsubscribe()
		obs.Set(resource.StatusClosed)

		assert.Equal(t, 1, notified, "observer must not be notified after unsubscribe")
		assert.Equal(t, resource.StatusClosed, obs.Get())
	})
}
