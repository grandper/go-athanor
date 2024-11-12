package nats_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	natsserver "github.com/grandper/go-athanor/resource/server/nats"
	"github.com/grandper/go-athanor/resource/server/nats/fixture"
)

func TestMiddlewareFunc(t *testing.T) {
	t.Run("should apply middlewares in registration order", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc)

		// The calls are appended on the dispatching goroutine and read by the test once the handler ran.
		var mu sync.Mutex
		var calls []string
		record := func(name string) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, name)
		}
		middleware := func(name string) natsserver.MiddlewareFunc {
			return func(next natsserver.HandlerFunc) natsserver.HandlerFunc {
				return func(r *natsserver.Request, publish natsserver.PublishFunc) error {
					record(name)
					return next(r, publish)
				}
			}
		}
		server.Use(middleware("first"), middleware("second"))

		handler := fixture.NewFakeHandlerFunc()
		handle := func(r *natsserver.Request, publish natsserver.PublishFunc) error {
			record("handler")
			return handler.Handle(r, publish)
		}
		require.NoError(t, server.HandleFunc("orders.created", "", handle))
		require.NoError(t, server.ListenAndServe())

		require.NoError(t, nc.Publish("orders.created", nil))

		handler.WaitForRequests(t, 1)
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, []string{"first", "second", "handler"}, calls)
	})
}
