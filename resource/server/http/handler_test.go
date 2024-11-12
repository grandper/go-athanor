package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpserver "github.com/grandper/go-athanor/resource/server/http"
	"github.com/grandper/go-athanor/resource/server/http/fixture"
)

func TestHandler(t *testing.T) {
	t.Run("should register its routes on the router", func(t *testing.T) {
		var handler httpserver.Handler = fixture.NewFakeHandler().WithRoute("/ping", http.StatusNoContent)

		router := mux.NewRouter()
		require.NoError(t, handler.Register(router))

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}
