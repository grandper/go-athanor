package http_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpserver "github.com/grandper/go-athanor/resource/server/http"
	"github.com/grandper/go-athanor/resource/server/http/fixture"
)

func TestStaticHandler(t *testing.T) {
	newRouter := func(t *testing.T) *mux.Router {
		t.Helper()
		staticDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(staticDir, "hello.txt"), []byte("Hello, World!"), 0o600))

		router := mux.NewRouter()
		require.NoError(t, httpserver.NewStaticHandler(staticDir, "/static/").Register(router))
		return router
	}

	t.Run("should serve files from the static directory under the URL prefix", func(t *testing.T) {
		rec := httptest.NewRecorder()

		newRouter(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/hello.txt", nil))

		fixture.AssertResponse(t, http.StatusOK, "text/plain; charset=utf-8", "Hello, World!", rec)
	})

	t.Run("should return 404 for a file that does not exist", func(t *testing.T) {
		rec := httptest.NewRecorder()

		newRouter(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/missing.txt", nil))

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}
