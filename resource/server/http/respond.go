package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const contentTypeJSON = "application/json"

var (
	// ErrWriteBody is returned when the body message cannot be written.
	ErrWriteBody = errors.New("body cannot be written")

	// ErrJSONEncoding is returned when the body message cannot be encoded in JSON format.
	ErrJSONEncoding = errors.New("body cannot be encoded as JSON")
)

// RespondWithJSON writes body to the response as JSON with the given status code.
func RespondWithJSON(w http.ResponseWriter, code int, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		_ = RespondWithError(w, http.StatusInternalServerError, ErrJSONEncoding)
		return fmt.Errorf("failed to respond with JSON: %w: %w", ErrJSONEncoding, err)
	}
	return writeJSON(w, code, data)
}

// RespondWithProtoJSON writes a protobuf message to the response as JSON with the given status code.
func RespondWithProtoJSON(w http.ResponseWriter, code int, message proto.Message) error {
	data, err := protojson.Marshal(message)
	if err != nil {
		_ = RespondWithError(w, http.StatusInternalServerError, ErrJSONEncoding)
		return fmt.Errorf("failed to respond with proto JSON: %w: %w", ErrJSONEncoding, err)
	}
	return writeJSON(w, code, data)
}

// RespondWithError writes the standard [StatusResponse] body for err with the given status code.
func RespondWithError(w http.ResponseWriter, code int, err error) error {
	return RespondWithStatus(w, code, err.Error())
}

// RespondWithStatus writes the standard [StatusResponse] body for message with the given status code.
func RespondWithStatus(w http.ResponseWriter, code int, message string) error {
	return NewStatusResponse(code, message).WriteJSON(w)
}

// RespondWithRawJSON validates and writes raw JSON data to the response with the given status code.
func RespondWithRawJSON(w http.ResponseWriter, code int, jsonData []byte) error {
	if !json.Valid(jsonData) {
		_ = RespondWithError(w, http.StatusInternalServerError, ErrJSONEncoding)
		return fmt.Errorf("failed to respond with raw JSON: %w", ErrJSONEncoding)
	}
	return writeJSON(w, code, bytes.TrimSpace(jsonData))
}

// codeOrOK defaults a zero status code to 200 OK, because http.ResponseWriter panics on a zero status code.
func codeOrOK(code int) int {
	if code == 0 {
		return http.StatusOK
	}
	return code
}

// writeJSON writes an already-encoded JSON payload with the given status code.
func writeJSON(w http.ResponseWriter, code int, data []byte) error {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(code)
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write the response: %w: %w", ErrWriteBody, err)
	}
	return nil
}

// WriteStatusText writes the standard text of an HTTP status code as a plain-text response.
func WriteStatusText(w http.ResponseWriter, status int) error {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write([]byte(http.StatusText(status))); err != nil {
		return fmt.Errorf("failed to write the status text: %w: %w", ErrWriteBody, err)
	}
	return nil
}
