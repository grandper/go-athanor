package nats

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/nats-io/nats.go"
)

const (
	contentTypeHeader = "Content-Type"
	statusHeader      = "Status"
	contentTypeJSON   = "application/json"
)

var (
	// ErrNoReplySubject is returned when replying to a message that carries no reply subject.
	ErrNoReplySubject = errors.New("request message has no reply subject")

	// ErrJSONEncoding is returned when the body message cannot be encoded in JSON format.
	ErrJSONEncoding = errors.New("body cannot be encoded as JSON")
)

// RespondWithJSON publishes body as JSON to the reply subject of requestMsg.
func RespondWithJSON(publish PublishFunc, requestMsg *nats.Msg, code int, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		_ = RespondWithError(publish, requestMsg, http.StatusInternalServerError, ErrJSONEncoding)
		return fmt.Errorf("failed to respond with JSON: %w: %w", ErrJSONEncoding, err)
	}
	return respond(publish, requestMsg, code, data)
}

// RespondWithError publishes the standard [StatusResponse] body for err to the reply subject of requestMsg.
func RespondWithError(publish PublishFunc, requestMsg *nats.Msg, code int, err error) error {
	return RespondWithStatus(publish, requestMsg, code, err.Error())
}

// RespondWithStatus publishes the standard [StatusResponse] body for message to the reply subject of requestMsg.
func RespondWithStatus(publish PublishFunc, requestMsg *nats.Msg, code int, message string) error {
	return RespondWithJSON(publish, requestMsg, code, NewStatusResponse(code, message))
}

// RespondWithRawJSON validates and publishes raw JSON data to the reply subject of requestMsg.
func RespondWithRawJSON(publish PublishFunc, requestMsg *nats.Msg, code int, jsonData []byte) error {
	if !json.Valid(jsonData) {
		_ = RespondWithError(publish, requestMsg, http.StatusInternalServerError, ErrJSONEncoding)
		return fmt.Errorf("failed to respond with raw JSON: %w", ErrJSONEncoding)
	}
	return respond(publish, requestMsg, code, bytes.TrimSpace(jsonData))
}

// respond publishes a JSON payload to the reply subject of requestMsg.
func respond(publish PublishFunc, requestMsg *nats.Msg, code int, data []byte) error {
	if requestMsg.Reply == "" {
		return fmt.Errorf("failed to reply to %q: %w", requestMsg.Subject, ErrNoReplySubject)
	}
	return publish(&nats.Msg{
		Subject: requestMsg.Reply,
		Header: nats.Header{
			contentTypeHeader: []string{contentTypeJSON},
			statusHeader:      []string{http.StatusText(code)},
		},
		Data: data,
	})
}
