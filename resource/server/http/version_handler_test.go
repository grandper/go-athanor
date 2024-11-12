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

func TestVersionHandler(t *testing.T) {
	info := httpserver.VersionInfo{
		Version:   "1.2.3",
		Commit:    "abc123",
		BuildDate: "2026-08-10T12:00:00Z",
	}

	t.Run("should expose the version information under the API version path", func(t *testing.T) {
		router := mux.NewRouter()
		require.NoError(t, httpserver.NewVersionHandler("v1", info).Register(router))

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/version", nil))

		fixture.AssertRawJSONResponse(t, http.StatusOK,
			`{"version":"1.2.3","commit":"abc123","build_date":"2026-08-10T12:00:00Z"}`, rec)
	})

	t.Run("should honor the API version given at construction", func(t *testing.T) {
		router := mux.NewRouter()
		require.NoError(t, httpserver.NewVersionHandler("v2", info).Register(router))

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v2/version", nil))

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("should only accept GET requests", func(t *testing.T) {
		router := mux.NewRouter()
		require.NoError(t, httpserver.NewVersionHandler("v1", info).Register(router))

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/version", nil))

		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})
}
