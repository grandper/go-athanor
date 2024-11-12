package fixture_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource/server/http/fixture"
)

func TestFailingResponseWriter(t *testing.T) {
	t.Run("fails every write with the configured error", func(t *testing.T) {
		w := fixture.NewFailingResponseWriter(assert.AnError)

		n, err := w.Write([]byte("body"))

		require.ErrorIs(t, err, assert.AnError)
		assert.Zero(t, n)
	})

	t.Run("should record the headers and the status code", func(t *testing.T) {
		w := fixture.NewFailingResponseWriter(assert.AnError)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTeapot)

		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.Equal(t, http.StatusTeapot, w.StatusCode())
	})
}
