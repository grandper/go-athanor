package resource_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grandper/go-athanor/resource"
)

func TestStatusObserverFunc(t *testing.T) {
	t.Run("should adapt an ordinary function to a StatusObserver", func(t *testing.T) {
		var got []resource.Status
		var observer resource.StatusObserver = resource.StatusObserverFunc(func(s resource.Status) {
			got = append(got, s)
		})

		observer.StatusChangedTo(resource.StatusHealthy)
		observer.StatusChangedTo(resource.StatusFailed)

		assert.Equal(t, []resource.Status{resource.StatusHealthy, resource.StatusFailed}, got)
	})
}
