package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource"
	"github.com/grandper/go-athanor/resource/fixture"
	httpserver "github.com/grandper/go-athanor/resource/server/http"
	httpfixture "github.com/grandper/go-athanor/resource/server/http/fixture"
)

// healthResponse mirrors the JSON payload written by HealthHandler.
type healthResponse struct {
	Status    bool              `json:"status"`
	Resources map[string]string `json:"resources"`
}

// serveHealth runs one request through a HealthHandler and decodes the response.
func serveHealth(t *testing.T, c *resource.List) (int, healthResponse) {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)

	httpserver.NewHealthHandler(c).ServeHTTP(w, r)

	result := w.Result()
	defer func() { _ = result.Body.Close() }()
	var response healthResponse
	require.NoError(t, json.NewDecoder(result.Body).Decode(&response))
	return result.StatusCode, response
}

func TestHealthHandler(t *testing.T) {
	ctx := context.Background()

	t.Run("should respond 200 with every resource status when all resources are operational", func(t *testing.T) {
		c := resource.NewList()
		require.NoError(t, c.Register(fixture.NewHealthyFakeManaged("db")))
		require.NoError(t, c.Register(fixture.NewHealthyFakeManaged("nats")))
		require.NoError(t, c.Init(ctx))

		code, response := serveHealth(t, c)

		assert.Equal(t, http.StatusOK, code)
		assert.True(t, response.Status)
		assert.Equal(t, map[string]string{"db": "HEALTHY", "nats": "HEALTHY"}, response.Resources)
	})

	t.Run("should respond 503 when a resource is not operational", func(t *testing.T) {
		c := resource.NewList()
		db := fixture.NewHealthyFakeManaged("db")
		require.NoError(t, c.Register(db))
		require.NoError(t, c.Init(ctx))
		db.Notify(resource.StatusUnhealthy)

		code, response := serveHealth(t, c)

		assert.Equal(t, http.StatusServiceUnavailable, code)
		assert.False(t, response.Status)
		assert.Equal(t, map[string]string{"db": "UNHEALTHY"}, response.Resources)
	})

	t.Run("should respond 200 for a list with no resources", func(t *testing.T) {
		c := resource.NewList()

		code, response := serveHealth(t, c)

		assert.Equal(t, http.StatusOK, code)
		assert.True(t, response.Status)
		assert.Empty(t, response.Resources)
	})

	t.Run("should serve the health on GET /health once registered", func(t *testing.T) {
		c := resource.NewList()
		require.NoError(t, c.Register(fixture.NewHealthyFakeManaged("db")))
		require.NoError(t, c.Init(ctx))
		router := mux.NewRouter()
		require.NoError(t, httpserver.NewHealthHandler(c).Register(router))

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

		httpfixture.AssertRawJSONResponse(t, http.StatusOK, `{"status":true,"resources":{"db":"HEALTHY"}}`, rec)
	})

	t.Run("should serve the health on the path set with WithPath", func(t *testing.T) {
		router := mux.NewRouter()
		require.NoError(t, httpserver.NewHealthHandler(resource.NewList()).WithPath("/healthz").Register(router))

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		assert.Equal(t, http.StatusOK, rec.Code)

		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("should only accept GET requests", func(t *testing.T) {
		router := mux.NewRouter()
		require.NoError(t, httpserver.NewHealthHandler(resource.NewList()).Register(router))

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/health", nil))

		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})
}
