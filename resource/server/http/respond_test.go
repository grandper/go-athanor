package http_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	httpserver "github.com/grandper/go-athanor/resource/server/http"
	"github.com/grandper/go-athanor/resource/server/http/fixture"
)

func TestRespondWithJSON(t *testing.T) {
	t.Run("should write the body as JSON with the given status code", func(t *testing.T) {
		rec := httptest.NewRecorder()

		err := httpserver.RespondWithJSON(rec, http.StatusCreated, map[string]string{"message": "created"})

		require.NoError(t, err)
		fixture.AssertJSONResponse(t, http.StatusCreated, map[string]string{"message": "created"}, rec)
	})

	t.Run("fails when the body cannot be encoded in JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()

		err := httpserver.RespondWithJSON(rec, http.StatusOK, make(chan int))

		require.ErrorIs(t, err, httpserver.ErrJSONEncoding)
		require.ErrorContains(t, err, "failed to respond with JSON")
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestRespondWithProtoJSON(t *testing.T) {
	t.Run("should write the proto message as JSON with the given status code", func(t *testing.T) {
		rec := httptest.NewRecorder()
		message, errNew := structpb.NewStruct(map[string]any{"version": "1.2.3"})
		require.NoError(t, errNew)

		err := httpserver.RespondWithProtoJSON(rec, http.StatusOK, message)

		require.NoError(t, err)
		fixture.AssertRawJSONResponse(t, http.StatusOK, `{"version":"1.2.3"}`, rec)
	})

	t.Run("fails when the message cannot be encoded in JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()

		// A Value carrying no kind is a valid Go value that protojson refuses to encode.
		err := httpserver.RespondWithProtoJSON(rec, http.StatusOK, &structpb.Value{})

		require.ErrorIs(t, err, httpserver.ErrJSONEncoding)
		require.ErrorContains(t, err, "failed to respond with proto JSON")
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestRespondWithError(t *testing.T) {
	t.Run("should write the standard error body with the given status code", func(t *testing.T) {
		rec := httptest.NewRecorder()

		err := httpserver.RespondWithError(rec, http.StatusBadRequest, errors.New("owner is required"))

		require.NoError(t, err)
		fixture.AssertRawJSONResponse(t, http.StatusBadRequest,
			`{"code":400,"status_text":"Bad Request","message":"owner is required"}`, rec)
	})
}

func TestRespondWithStatus(t *testing.T) {
	t.Run("should write the standard status body with the given status code", func(t *testing.T) {
		rec := httptest.NewRecorder()

		err := httpserver.RespondWithStatus(rec, http.StatusOK, "Success")

		require.NoError(t, err)
		fixture.AssertRawJSONResponse(t, http.StatusOK, `{"code":200,"status_text":"OK","message":"Success"}`, rec)
	})
}

func TestRespondWithRawJSON(t *testing.T) {
	t.Run("should write the raw JSON data trimmed of surrounding whitespace", func(t *testing.T) {
		rec := httptest.NewRecorder()

		err := httpserver.RespondWithRawJSON(rec, http.StatusOK, []byte("  {\"key\":\"value\"}\n"))

		require.NoError(t, err)
		fixture.AssertRawJSONResponse(t, http.StatusOK, `{"key":"value"}`, rec)
	})

	t.Run("fails when the data is not valid JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()

		err := httpserver.RespondWithRawJSON(rec, http.StatusOK, []byte(`{"key":`))

		require.ErrorIs(t, err, httpserver.ErrJSONEncoding)
		require.ErrorContains(t, err, "failed to respond with raw JSON")
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestWriteStatusText(t *testing.T) {
	t.Run("should write the standard status text as plain text", func(t *testing.T) {
		w := httptest.NewRecorder()

		require.NoError(t, httpserver.WriteStatusText(w, http.StatusNotFound))

		fixture.AssertResponse(t, http.StatusNotFound, "text/plain; charset=utf-8", "Not Found", w)
	})

	t.Run("should write the status text of a success as well", func(t *testing.T) {
		w := httptest.NewRecorder()

		require.NoError(t, httpserver.WriteStatusText(w, http.StatusOK))

		assert.Equal(t, "OK", w.Body.String())
	})

	t.Run("fails when the response cannot be written", func(t *testing.T) {
		err := httpserver.WriteStatusText(fixture.NewFailingResponseWriter(assert.AnError), http.StatusOK)

		require.ErrorIs(t, err, httpserver.ErrWriteBody)
		require.ErrorIs(t, err, assert.AnError)
		require.ErrorContains(t, err, "failed to write the status text")
	})
}
