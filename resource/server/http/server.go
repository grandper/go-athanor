package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"github.com/grandper/go-athanor/resource"
)

const (
	// DefaultName is the default name of the HTTP server resource.
	DefaultName = "http-server"

	readTimeoutDuration  = 2 * time.Second
	writeTimeoutDuration = 2 * time.Second
	idleTimeoutDuration  = 3 * time.Second
)

var (
	// ErrEmptyBindInterface is returned when the bind interface is empty.
	ErrEmptyBindInterface = errors.New("bind interface is empty")

	// ErrBindPortNotInitialized is returned when the bind port is not initialized.
	ErrBindPortNotInitialized = errors.New("bind port is not initialized")

	// ErrServerAlreadyStarted is returned when Init is called on a server that already started,
	// whether it is still running or was shut down.
	ErrServerAlreadyStarted = errors.New("HTTP server already started")
)

// Server is a managed resource serving the routes registered by its handlers.
type Server struct {
	name     string
	config   *Config
	handlers []Handler
	status   *resource.ObservableStatus

	mu         sync.Mutex
	started    bool // claimed by the first successful Init, for good
	httpServer *http.Server
	done       chan struct{} // closed when the serve goroutine exits
	serveErr   error         // first unexpected serve error, read by Shutdown
}

// NewServer creates an HTTP server resource serving the routes registered by the handlers.
func NewServer(config *Config, handlers ...Handler) *Server {
	return &Server{
		name:     DefaultName,
		config:   config,
		handlers: handlers,
		status:   resource.NewObservableStatus(resource.StatusInitializing),
	}
}

// WithName overrides the name identifying the server inside a resource.List.
func (s *Server) WithName(name string) *Server {
	s.name = name
	return s
}

// Name implements resource.Managed.
func (s *Server) Name() string { return s.name }

// Status implements resource.Managed.
func (s *Server) Status() *resource.ObservableStatus { return s.status }

// Init implements resource.Managed. It binds the listener and serves in the background.
// A server starts once: Init fails with ErrServerAlreadyStarted on a running server and on a server that was
// shut down, while an Init that failed can be retried.
func (s *Server) Init(ctx context.Context) error {
	if !s.claimStart() {
		// The running server is left untouched: a second listener and serve goroutine would leak.
		return fmt.Errorf("failed to init the HTTP server: %w", ErrServerAlreadyStarted)
	}
	if err := s.start(ctx); err != nil {
		s.releaseStart()
		s.status.Set(resource.StatusFailed)
		return err
	}
	return nil
}

// start validates the configuration, binds the listener, and serves in the background.
func (s *Server) start(ctx context.Context) error {
	if s.config.BindInterface == "" {
		return fmt.Errorf("failed to init the HTTP server: %w", ErrEmptyBindInterface)
	}
	if s.config.BindPort == 0 {
		return fmt.Errorf("failed to init the HTTP server: %w", ErrBindPortNotInitialized)
	}

	router := mux.NewRouter()
	for _, handler := range s.handlers {
		if err := handler.Register(router); err != nil {
			return fmt.Errorf("failed to register an HTTP handler: %w", err)
		}
	}

	// Bind synchronously so address errors are returned by Init.
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", s.config.Address())
	if err != nil {
		return fmt.Errorf("failed to listen on %q: %w", s.config.Address(), err)
	}

	httpServer := &http.Server{
		Addr:         s.config.Address(),
		Handler:      router,
		ReadTimeout:  readTimeoutDuration,
		WriteTimeout: writeTimeoutDuration,
		IdleTimeout:  idleTimeoutDuration,
	}

	done := make(chan struct{})
	s.mu.Lock()
	s.httpServer = httpServer
	s.done = done
	s.mu.Unlock()

	s.status.Set(resource.StatusHealthy)

	go func() {
		defer close(done)
		errServe := s.serve(httpServer, listener)
		if errServe != nil && !errors.Is(errServe, http.ErrServerClosed) {
			s.mu.Lock()
			s.serveErr = errServe
			s.mu.Unlock()
			s.status.Set(resource.StatusFailed)
		}
	}()
	return nil
}

// claimStart marks the server as started and reports whether the caller is the one that did it.
func (s *Server) claimStart() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return false
	}
	s.started = true
	return true
}

// releaseStart lets Init be called again after it failed.
func (s *Server) releaseStart() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = false
}

// serve runs the accept loop on the bound listener, with TLS when available.
func (s *Server) serve(httpServer *http.Server, listener net.Listener) error {
	if s.config.TLSIsAvailable() {
		return httpServer.ServeTLS(listener, s.config.TLSCertFile, s.config.TLSKeyFile)
	}
	return httpServer.Serve(listener)
}

// Shutdown implements resource.Managed. It shuts the server down gracefully.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	httpServer, done := s.httpServer, s.done
	s.mu.Unlock()

	if httpServer == nil {
		// Init never succeeded: nothing to stop.
		s.status.Set(resource.StatusClosed)
		return nil
	}

	shutdownErr := httpServer.Shutdown(ctx)
	<-done

	s.mu.Lock()
	serveErr := s.serveErr
	s.mu.Unlock()
	switch {
	case serveErr != nil:
		s.status.Set(resource.StatusFailed)
		return fmt.Errorf("failed to serve HTTP: %w", serveErr)
	case shutdownErr != nil:
		s.status.Set(resource.StatusFailed)
		return fmt.Errorf("failed to shut down the HTTP server: %w", shutdownErr)
	}
	s.status.Set(resource.StatusClosed)
	return nil
}

// Server implements the resource.Managed interface.
var _ resource.Managed = (*Server)(nil)
