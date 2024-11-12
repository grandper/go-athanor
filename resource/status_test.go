package resource_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grandper/go-athanor/resource"
)

func TestStatus(t *testing.T) {
	cases := []struct {
		s           resource.Status
		str         string
		operational bool
		terminal    bool
		failed      bool
	}{
		{resource.StatusInitializing, "INITIALIZING", false, false, false},
		{resource.StatusHealthy, "HEALTHY", true, false, false},
		{resource.StatusDegraded, "DEGRADED", true, false, false},
		{resource.StatusUnhealthy, "UNHEALTHY", false, false, false},
		{resource.StatusFailed, "FAILED", false, true, true},
		{resource.StatusClosed, "CLOSED", false, true, false},
		{resource.Status(99), "UNKNOWN", false, false, false},
	}

	t.Run("implements the stringer interface", func(t *testing.T) {
		for _, tc := range cases {
			assert.Equal(t, tc.str, tc.s.String())
		}
	})

	t.Run("should report whether the resource is operational", func(t *testing.T) {
		for _, tc := range cases {
			assert.Equal(t, tc.operational, tc.s.IsOperational(), "IsOperational(%s)", tc.s)
		}
	})

	t.Run("should report whether the resource has stopped for good", func(t *testing.T) {
		for _, tc := range cases {
			assert.Equal(t, tc.terminal, tc.s.IsTerminal(), "IsTerminal(%s)", tc.s)
		}
	})

	t.Run("should report whether the resource has failed", func(t *testing.T) {
		for _, tc := range cases {
			assert.Equal(t, tc.failed, tc.s.IsFailed(), "IsFailed(%s)", tc.s)
		}
	})
}
