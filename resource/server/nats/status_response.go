package nats

import "net/http"

// StatusResponse is the standard JSON body for a status message.
type StatusResponse struct {
	// Code is the HTTP status code naming the outcome of the request.
	Code int `json:"code"`

	// StatusText is the standard text of the status code, such as "Not Found".
	StatusText string `json:"status_text"`

	// Message is the message reported to the requester.
	Message string `json:"message"`
}

// NewStatusResponse builds a StatusResponse from an HTTP status code and a message.
func NewStatusResponse(code int, message string) StatusResponse {
	return StatusResponse{
		Code:       code,
		StatusText: http.StatusText(code),
		Message:    message,
	}
}
