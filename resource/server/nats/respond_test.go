package nats_test

import (
	"net/http"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	natsserver "github.com/grandper/go-athanor/resource/server/nats"
	"github.com/grandper/go-athanor/resource/server/nats/fixture"
)

func TestRespondWithJSON(t *testing.T) {
	t.Run("should publish the body as JSON to the reply subject", func(t *testing.T) {
		publisher := fixture.NewFakePublishFunc()
		requestMsg := &nats.Msg{Subject: "config.get", Reply: "inbox"}

		err := natsserver.RespondWithJSON(publisher.Publish, requestMsg, http.StatusOK,
			map[string]string{"name": "payments"})

		require.NoError(t, err)
		published := publisher.Published()
		require.Len(t, published, 1)
		assert.Equal(t, "inbox", published[0].Subject)
		assert.JSONEq(t, `{"name":"payments"}`, string(published[0].Data))
		assert.Equal(t, "application/json", published[0].Header.Get("Content-Type"))
		assert.Equal(t, "OK", published[0].Header.Get("Status"))
	})

	t.Run("fails when the request has no reply subject", func(t *testing.T) {
		publisher := fixture.NewFakePublishFunc()
		requestMsg := &nats.Msg{Subject: "config.get"}

		err := natsserver.RespondWithJSON(publisher.Publish, requestMsg, http.StatusOK, "body")

		require.ErrorIs(t, err, natsserver.ErrNoReplySubject)
		require.ErrorContains(t, err, "failed to reply")
	})

	t.Run("fails when the body cannot be encoded in JSON", func(t *testing.T) {
		publisher := fixture.NewFakePublishFunc()
		requestMsg := &nats.Msg{Subject: "config.get", Reply: "inbox"}

		err := natsserver.RespondWithJSON(publisher.Publish, requestMsg, http.StatusOK, make(chan int))

		require.ErrorIs(t, err, natsserver.ErrJSONEncoding)
		require.ErrorContains(t, err, "failed to respond with JSON")
		assertEncodingErrorPublished(t, publisher)
	})

	t.Run("fails when the reply cannot be published", func(t *testing.T) {
		publisher := fixture.NewFakePublishFunc().FailWith(assert.AnError)
		requestMsg := &nats.Msg{Subject: "config.get", Reply: "inbox"}

		err := natsserver.RespondWithJSON(publisher.Publish, requestMsg, http.StatusOK, "body")

		require.ErrorIs(t, err, assert.AnError)
	})
}

func TestRespondWithError(t *testing.T) {
	t.Run("should publish the standard error body to the reply subject", func(t *testing.T) {
		publisher := fixture.NewFakePublishFunc()
		requestMsg := &nats.Msg{Subject: "config.get", Reply: "inbox"}

		err := natsserver.RespondWithError(publisher.Publish, requestMsg, http.StatusBadRequest,
			assert.AnError)

		require.NoError(t, err)
		published := publisher.Published()
		require.Len(t, published, 1)
		assert.Equal(t, "inbox", published[0].Subject)
		assert.JSONEq(t,
			`{"code":400,"status_text":"Bad Request","message":"`+assert.AnError.Error()+`"}`,
			string(published[0].Data))
		assert.Equal(t, "Bad Request", published[0].Header.Get("Status"))
	})
}

func TestRespondWithStatus(t *testing.T) {
	t.Run("should publish the standard status body to the reply subject", func(t *testing.T) {
		publisher := fixture.NewFakePublishFunc()
		requestMsg := &nats.Msg{Subject: "config.get", Reply: "inbox"}

		err := natsserver.RespondWithStatus(publisher.Publish, requestMsg, http.StatusOK, "Success")

		require.NoError(t, err)
		published := publisher.Published()
		require.Len(t, published, 1)
		assert.Equal(t, "inbox", published[0].Subject)
		assert.JSONEq(t, `{"code":200,"status_text":"OK","message":"Success"}`, string(published[0].Data))
		assert.Equal(t, "OK", published[0].Header.Get("Status"))
	})
}

func TestRespondWithRawJSON(t *testing.T) {
	t.Run("should publish the raw JSON data trimmed of surrounding whitespace", func(t *testing.T) {
		publisher := fixture.NewFakePublishFunc()
		requestMsg := &nats.Msg{Subject: "config.get", Reply: "inbox"}
		trimmed := `{"name":"payments"}`

		err := natsserver.RespondWithRawJSON(publisher.Publish, requestMsg, http.StatusOK,
			[]byte("  "+trimmed+"\n"))

		require.NoError(t, err)
		published := publisher.Published()
		require.Len(t, published, 1)
		assert.Equal(t, trimmed, string(published[0].Data))
	})

	t.Run("fails when the data is not valid JSON", func(t *testing.T) {
		publisher := fixture.NewFakePublishFunc()
		requestMsg := &nats.Msg{Subject: "config.get", Reply: "inbox"}

		err := natsserver.RespondWithRawJSON(publisher.Publish, requestMsg, http.StatusOK, []byte(`{"name":`))

		require.ErrorIs(t, err, natsserver.ErrJSONEncoding)
		require.ErrorContains(t, err, "failed to respond with raw JSON")
		assertEncodingErrorPublished(t, publisher)
	})
}

// assertEncodingErrorPublished asserts that the requester was answered a 500 error naming the encoding failure.
func assertEncodingErrorPublished(t *testing.T, publisher *fixture.FakePublishFunc) {
	t.Helper()

	published := publisher.Published()
	require.Len(t, published, 1)
	assert.Equal(t, "inbox", published[0].Subject)
	assert.JSONEq(t,
		`{"code":500,"status_text":"Internal Server Error","message":"`+natsserver.ErrJSONEncoding.Error()+`"}`,
		string(published[0].Data))
	assert.Equal(t, "Internal Server Error", published[0].Header.Get("Status"))
}
