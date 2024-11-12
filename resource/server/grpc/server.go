package grpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"google.golang.org/grpc"

	"github.com/grandper/go-athanor/resource"
)

// DefaultName is the default name of the gRPC server.
const DefaultName = "grpc-server"

var (
	// ErrEmptyBindInterface is returned when the bind interface is empty.
	ErrEmptyBindInterface = errors.New("bind interface is empty")

	// ErrBindPortNotInitialized is returned when the bind port is not initialized.
	ErrBindPortNotInitialized = errors.New("bind port is not initialized")

	// ErrServerAlreadyStarted is returned when Init is called on a server that already started,
	// whether it is still running or was shut down.
	ErrServerAlreadyStarted = errors.New("gRPC server already started")
)

// Server is a managed resource serving the gRPC services registered by its handlers.
type Server struct {
	name     string
	config   *Config
	handlers []Handler
	listener net.Listener
	status   *resource.ObservableStatus

	mu         sync.Mutex
	started    bool // claimed by the first successful Init, for good
	grpcServer *grpc.Server
	done       chan struct{} // closed when the serve goroutine exits
	serveErr   error         // first unexpected serve error, read by Shutdown
}

// NewServer creates a gRPC server resource serving the services registered by the handlers.
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

// WithListener makes the server serve on listener instead of binding the address of its configuration.
// The server owns the listener from then on and closes it when it stops serving.
func (s *Server) WithListener(listener net.Listener) *Server {
	s.listener = listener
	return s
}

// Name implements resource.Managed.
func (s *Server) Name() string { return s.name }

// Status implements resource.Managed.
func (s *Server) Status() *resource.ObservableStatus { return s.status }

// Init implements resource.Managed. It starts serving in the background.
// A server starts once: Init fails with ErrServerAlreadyStarted on a running server and on a server that was
// shut down, while an Init that failed can be retried.
func (s *Server) Init(ctx context.Context) error {
	if !s.claimStart() {
		// The running server is left untouched: a second listener and serve goroutine would leak.
		return fmt.Errorf("failed to init the gRPC server: %w", ErrServerAlreadyStarted)
	}

	if err := s.start(ctx); err != nil {
		s.releaseStart()
		s.status.Set(resource.StatusFailed)
		return err
	}
	return nil
}

// start registers the handlers, binds the listener, and serves in the background.
func (s *Server) start(ctx context.Context) error {
	// Register before binding, so a failed registration leaves no listener behind.
	grpcServer := grpc.NewServer()
	for _, handler := range s.handlers {
		if err := handler.Register(grpcServer); err != nil {
			return fmt.Errorf("failed to register a gRPC handler: %w", err)
		}
	}

	listener, err := s.listen(ctx)
	if err != nil {
		return err
	}

	done := make(chan struct{})
	s.mu.Lock()
	s.grpcServer = grpcServer
	s.done = done
	s.mu.Unlock()

	s.status.Set(resource.StatusHealthy)

	go func() {
		defer close(done)
		errServe := grpcServer.Serve(listener)
		if errServe != nil && !errors.Is(errServe, grpc.ErrServerStopped) {
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

// listen returns the listener set with WithListener, or binds the address of the configuration.
func (s *Server) listen(ctx context.Context) (net.Listener, error) {
	if s.listener != nil {
		return s.listener, nil
	}
	if s.config.BindInterface == "" {
		return nil, fmt.Errorf("failed to init the gRPC server: %w", ErrEmptyBindInterface)
	}
	if s.config.BindPort == 0 {
		return nil, fmt.Errorf("failed to init the gRPC server: %w", ErrBindPortNotInitialized)
	}

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", s.config.Address())
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %q: %w", s.config.Address(), err)
	}
	return listener, nil
}

// Shutdown implements resource.Managed. It stops the server gracefully, or immediately when ctx expires.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	grpcServer, done := s.grpcServer, s.done
	s.mu.Unlock()

	if grpcServer == nil {
		// Init never succeeded: nothing to stop.
		s.status.Set(resource.StatusClosed)
		return nil
	}

	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-ctx.Done():
		grpcServer.Stop()
		<-stopped
	}
	<-done

	s.mu.Lock()
	serveErr := s.serveErr
	s.mu.Unlock()
	if serveErr != nil {
		s.status.Set(resource.StatusFailed)
		return fmt.Errorf("failed to serve gRPC: %w", serveErr)
	}
	s.status.Set(resource.StatusClosed)
	return nil
}

// Server implements the resource.Managed interface.
var _ resource.Managed = (*Server)(nil)
