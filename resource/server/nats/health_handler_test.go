package nats_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	athanorfixture "github.com/grandper/go-athanor/fixture"
	"github.com/grandper/go-athanor/resource"
	resourcefixture "github.com/grandper/go-athanor/resource/fixture"
	natsserver "github.com/grandper/go-athanor/resource/server/nats"
	"github.com/grandper/go-athanor/resource/server/nats/fixture"
)

// requestHealth serves a HealthHandler on an embedded NATS server and returns its reply to one request on subject.
func requestHealth(t *testing.T, handler *natsserver.HealthHandler, subject string) *nats.Msg {
	t.Helper()
	nc := fixture.ConnectToEmbeddedServer(t)
	server := natsserver.NewServer(nc, handler)
	require.NoError(t, server.Init(context.Background()))
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	reply, err := nc.Request(subject, nil, requestTimeout)
	require.NoError(t, err)
	return reply
}

func TestHealthHandler(t *testing.T) {
	ctx := context.Background()

	t.Run("should reply OK with every resource status when all resources are operational", func(t *testing.T) {
		resources := resource.NewList()
		require.NoError(t, resources.Register(resourcefixture.NewHealthyFakeManaged("db")))
		require.NoError(t, resources.Register(resourcefixture.NewHealthyFakeManaged("cache")))
		require.NoError(t, resources.Init(ctx))

		reply := requestHealth(t, natsserver.NewHealthHandler(resources), "health")

		assert.Equal(t, "OK", reply.Header.Get("Status"))
		assert.Equal(t, "application/json", reply.Header.Get("Content-Type"))
		assert.JSONEq(t, `{"status":true,"resources":{"db":"HEALTHY","cache":"HEALTHY"}}`, string(reply.Data))
	})

	t.Run("should reply Service Unavailable when a resource is not operational", func(t *testing.T) {
		resources := resource.NewList()
		db := resourcefixture.NewHealthyFakeManaged("db")
		require.NoError(t, resources.Register(db))
		require.NoError(t, resources.Init(ctx))
		db.Notify(resource.StatusUnhealthy)

		reply := requestHealth(t, natsserver.NewHealthHandler(resources), "health")

		assert.Equal(t, "Service Unavailable", reply.Header.Get("Status"))
		assert.JSONEq(t, `{"status":false,"resources":{"db":"UNHEALTHY"}}`, string(reply.Data))
	})

	t.Run("should reply OK for a list with no resources", func(t *testing.T) {
		reply := requestHealth(t, natsserver.NewHealthHandler(resource.NewList()), "health")

		assert.Equal(t, "OK", reply.Header.Get("Status"))
		assert.JSONEq(t, `{"status":true,"resources":{}}`, string(reply.Data))
	})

	t.Run("should serve the health on the subject set with WithSubject", func(t *testing.T) {
		handler := natsserver.NewHealthHandler(resource.NewList()).WithSubject("my-service.health")

		reply := requestHealth(t, handler, "my-service.health")

		assert.Equal(t, "OK", reply.Header.Get("Status"))
	})

	t.Run("fails to register when the subject is empty", func(t *testing.T) {
		server := natsserver.NewServer(fixture.ConnectToEmbeddedServer(t))

		err := natsserver.NewHealthHandler(resource.NewList()).WithSubject("").Register(server)

		assert.ErrorIs(t, err, natsserver.ErrEmptySubject)
	})

	t.Run("fails when the request has no reply subject", func(t *testing.T) {
		nc := fixture.ConnectToEmbeddedServer(t)
		server := natsserver.NewServer(nc, natsserver.NewHealthHandler(resource.NewList()))
		reported := make(chan error, 1)
		server.OnError(func(err error) { reported <- err })
		require.NoError(t, server.Init(ctx))

		require.NoError(t, nc.Publish("health", nil))

		err := athanorfixture.Receive(t, reported, requestTimeout, "the error handler was never called")
		assert.ErrorIs(t, err, natsserver.ErrNoReplySubject)
	})
}
