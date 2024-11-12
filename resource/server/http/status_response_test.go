package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpserver "github.com/grandper/go-athanor/resource/server/http"
	"github.com/grandper/go-athanor/resource/server/http/fixture"
)

func TestStatusResponse(t *testing.T) {
	t.Run("should carry the HTTP status code, its text, and the message", func(t *testing.T) {
		response := httpserver.NewStatusResponse(http.StatusOK, "Alive")

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "OK", response.StatusText)
		assert.Equal(t, "Alive", response.Message)
	})

	t.Run("should marshal to the standard status JSON body", func(t *testing.T) {
		response := httpserver.NewStatusResponse(http.StatusOK, "Ready")

		data, err := json.Marshal(response)

		require.NoError(t, err)
		assert.JSONEq(t, `{"code":200,"status_text":"OK","message":"Ready"}`, string(data))
	})
}

func TestStatusResponseWriteJSON(t *testing.T) {
	t.Run("should write the body as JSON with its own status code", func(t *testing.T) {
		rec := httptest.NewRecorder()

		err := httpserver.NewStatusResponse(http.StatusServiceUnavailable, "Draining").WriteJSON(rec)

		require.NoError(t, err)
		fixture.AssertRawJSONResponse(t, http.StatusServiceUnavailable,
			`{"code":503,"status_text":"Service Unavailable","message":"Draining"}`, rec)
	})

	t.Run("should report a zero code as 200 OK", func(t *testing.T) {
		rec := httptest.NewRecorder()

		err := httpserver.StatusResponse{Message: "no code"}.WriteJSON(rec)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("fails when the body cannot be written", func(t *testing.T) {
		w := fixture.NewFailingResponseWriter(assert.AnError)

		err := httpserver.NewStatusResponse(http.StatusOK, "Alive").WriteJSON(w)

		require.ErrorIs(t, err, httpserver.ErrWriteBody)
		require.ErrorContains(t, err, "failed to write the response")
		assert.ErrorIs(t, err, assert.AnError)
	})
}
