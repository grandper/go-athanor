package fixture_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpserver "github.com/grandper/go-athanor/resource/server/http"
	"github.com/grandper/go-athanor/resource/server/http/fixture"
)

func TestFakeHandler(t *testing.T) {
	t.Run("should record that it was registered", func(t *testing.T) {
		handler := fixture.NewFakeHandler()
		assert.False(t, handler.WasRegistered())

		require.NoError(t, handler.Register(mux.NewRouter()))

		assert.True(t, handler.WasRegistered())
	})

	t.Run("should delegate the registration to the configured function", func(t *testing.T) {
		handler := fixture.NewFakeHandler().WithRegisterFunc(func(router *mux.Router) error {
			router.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
			return nil
		})

		router := mux.NewRouter()
		require.NoError(t, handler.Register(router))

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("should answer the configured routes with their status code", func(t *testing.T) {
		handler := fixture.NewFakeHandler().
			WithRoute("/ping", http.StatusNoContent).
			WithRoute("/teapot", http.StatusTeapot)
		router := mux.NewRouter()

		require.NoError(t, handler.Register(router))

		for path, expectedCode := range map[string]int{"/ping": http.StatusNoContent, "/teapot": http.StatusTeapot} {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			assert.Equal(t, expectedCode, rec.Code, path)
		}
	})

	t.Run("fails the registration with the configured error", func(t *testing.T) {
		errRegister := errors.New("registration failed")
		handler := fixture.NewFakeHandler().FailRegisterWith(errRegister)

		err := handler.Register(mux.NewRouter())

		require.ErrorIs(t, err, errRegister)
		assert.True(t, handler.WasRegistered())
	})
}

func TestPanickingHandler(t *testing.T) {
	t.Run("should register a route that panics with the given value", func(t *testing.T) {
		router := mux.NewRouter()
		require.NoError(t, fixture.NewPanickingHandler("/orders/42", "the order has no line").Register(router))

		assert.PanicsWithValue(t, "the order has no line", func() {
			router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/orders/42", nil))
		})
	})

	t.Run("should leave the other paths alone", func(t *testing.T) {
		router := mux.NewRouter()
		require.NoError(t, fixture.NewPanickingHandler("/orders/42", "boom").Register(router))
		rec := httptest.NewRecorder()

		assert.NotPanics(t, func() {
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/orders/43", nil))
		})
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

// serveHijackingRoute starts a server whose route /chat is handled by handler, and returns its address.
// The server is shut down when the test ends.
func serveHijackingRoute(t *testing.T, handler *fixture.HijackingHandler) string {
	t.Helper()
	ctx := context.Background()
	config := &httpserver.Config{BindInterface: "127.0.0.1"}
	server := httpserver.NewServer(config, handler)
	require.NoError(t, fixture.InitOnFreePort(ctx, t, config, server.Init))
	t.Cleanup(func() { _ = server.Shutdown(ctx) })
	return config.Address()
}

func TestHijackingHandler(t *testing.T) {
	t.Run("should take the connection over and keep it open", func(t *testing.T) {
		handler := fixture.NewHijackingHandler("/chat")
		address := serveHijackingRoute(t, handler)

		conn := fixture.OpenHijackedConnection(t, address, "/chat")

		require.Eventually(t, func() bool { return handler.Hijacked() == 1 }, time.Second, 5*time.Millisecond)
		fixture.AssertStaysOpen(t, conn, 50*time.Millisecond)
	})

	t.Run("should close every connection it took over with CloseAll", func(t *testing.T) {
		handler := fixture.NewHijackingHandler("/chat")
		address := serveHijackingRoute(t, handler)
		first := fixture.OpenHijackedConnection(t, address, "/chat")
		second := fixture.OpenHijackedConnection(t, address, "/chat")
		require.Eventually(t, func() bool { return handler.Hijacked() == 2 }, time.Second, 5*time.Millisecond)

		handler.CloseAll()

		fixture.WaitForClosedConnection(t, first)
		fixture.WaitForClosedConnection(t, second)
		assert.Zero(t, handler.Hijacked())
	})
}
