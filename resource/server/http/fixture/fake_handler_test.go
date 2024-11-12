package fixture_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
