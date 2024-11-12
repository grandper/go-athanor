package nats

// Handler registers message subscriptions on a server.
type Handler interface {
	// Register registers the subscriptions of the handler on server with [Server.HandleFunc].
	// An error makes the initialization of the server fail.
	Register(server *Server) error
}
