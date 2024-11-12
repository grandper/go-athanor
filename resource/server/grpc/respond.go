package grpc

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RespondWithError returns the gRPC status error a handler sends back when handling a call fails with err.
// The error carries the given gRPC code and the message of err.
func RespondWithError(code codes.Code, err error) error {
	return status.Error(code, err.Error())
}
