package grpc

import "google.golang.org/grpc"

// Handler registers gRPC services on a server.
type Handler interface {
	// Register registers the services of the handler on server, before the server starts serving.
	// An error makes the initialization of the server fail.
	Register(server *grpc.Server) error
}
