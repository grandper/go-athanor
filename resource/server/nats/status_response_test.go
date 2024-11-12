package nats_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	natsserver "github.com/grandper/go-athanor/resource/server/nats"
)

func TestStatusResponse(t *testing.T) {
	t.Run("should carry the HTTP status code, its text, and the message", func(t *testing.T) {
		response := natsserver.NewStatusResponse(http.StatusOK, "Alive")

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "OK", response.StatusText)
		assert.Equal(t, "Alive", response.Message)
	})

	t.Run("should marshal to the standard status JSON body", func(t *testing.T) {
		response := natsserver.NewStatusResponse(http.StatusOK, "Ready")

		data, err := json.Marshal(response)

		require.NoError(t, err)
		assert.JSONEq(t, `{"code":200,"status_text":"OK","message":"Ready"}`, string(data))
	})
}
