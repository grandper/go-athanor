package fixture_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/grandper/go-athanor/resource/server/http/fixture"
)

func writeJSONResponse(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	rec.WriteHeader(http.StatusOK)
	_, _ = rec.WriteString(body)
	return rec
}

func TestAssertJSONResponse(t *testing.T) {
	t.Run("should accept a response matching the JSON encoding of the expected body", func(t *testing.T) {
		rec := writeJSONResponse(t, `{"message":"pong"}`)

		fixture.AssertJSONResponse(t, http.StatusOK, map[string]string{"message": "pong"}, rec)
	})
}

func TestAssertRawJSONResponse(t *testing.T) {
	t.Run("should accept a response JSON-equal to the expected document", func(t *testing.T) {
		rec := writeJSONResponse(t, `{"a":1,"b":2}`)

		fixture.AssertRawJSONResponse(t, http.StatusOK, `{"b":2,"a":1}`, rec)
	})
}

func TestAssertResponse(t *testing.T) {
	t.Run("should accept a response with the expected code, content type, and exact body", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rec.Header().Set("Content-Type", "text/plain")
		rec.WriteHeader(http.StatusTeapot)
		_, _ = rec.WriteString("short and stout")

		fixture.AssertResponse(t, http.StatusTeapot, "text/plain", "short and stout", rec)
	})
}

func TestAssertEventuallyResponds(t *testing.T) {
	t.Run("should accept a server answering with the expected status code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		fixture.AssertEventuallyResponds(t, server.URL, http.StatusNoContent)
	})

	t.Run("should keep polling until the expected status code shows up", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		fixture.AssertEventuallyResponds(t, server.URL, http.StatusOK)
	})
}
