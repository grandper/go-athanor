package http

import "github.com/gorilla/mux"

// Handler is an HTTP handler that registers its routes on the server's router.
type Handler interface {
	// Register registers the routes of the handler on router, before the server starts serving.
	// An error makes the initialization of the server fail.
	Register(router *mux.Router) error
}
