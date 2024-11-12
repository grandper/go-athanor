package nats

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/nats-io/nats.go"

	"github.com/grandper/go-athanor/resource"
)

// DefaultName is the default name of the NATS server.
const DefaultName = "nats-server"

var (
	// ErrEmptySubject is returned when a handler is registered with an empty subject.
	ErrEmptySubject = errors.New("subject is empty")

	// ErrServerAlreadyStarted is returned when a handler is registered, or the server is started again, after
	// the server started listening, whether it is still running or was shut down.
	ErrServerAlreadyStarted = errors.New("NATS server already started")

	// ErrNilNATSConnection is returned when the NATS connection is nil.
	ErrNilNATSConnection = errors.New("NATS connection is nil")
)

// registration is a handler waiting for the server to start listening.
type registration struct {
	subject string
	queue   string
	handler HandlerFunc
}

// Server is a managed resource dispatching NATS messages to registered handler functions.
type Server struct {
	name     string
	nc       *nats.Conn
	handlers []Handler
	status   *resource.ObservableStatus

	mu            sync.Mutex
	middlewares   []MiddlewareFunc
	registrations []registration
	subscriptions []*nats.Subscription
	errorHandler  func(err error)
	started       bool
	cancelServing context.CancelFunc // cancels the context of the requests
}

// NewServer returns a Server consuming messages from the NATS connection nc and dispatching to the handlers.
func NewServer(nc *nats.Conn, handlers ...Handler) *Server {
	return &Server{
		name:     DefaultName,
		nc:       nc,
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

// Init implements resource.Managed. It registers the handlers and starts listening.
// A server starts once: Init fails with ErrServerAlreadyStarted on a running server and on a server that was
// shut down, while an Init that failed can be retried.
func (s *Server) Init(_ context.Context) error {
	if s.nc == nil {
		s.status.Set(resource.StatusFailed)
		return fmt.Errorf("failed to init the NATS server: %w", ErrNilNATSConnection)
	}
	if s.isStarted() {
		// The running server is left untouched: registering the handlers again would fail it.
		return fmt.Errorf("failed to init the NATS server: %w", ErrServerAlreadyStarted)
	}
	for _, handler := range s.handlers {
		if err := handler.Register(s); err != nil {
			s.status.Set(resource.StatusFailed)
			return fmt.Errorf("failed to register a NATS handler: %w", err)
		}
	}
	if err := s.ListenAndServe(); err != nil {
		s.status.Set(resource.StatusFailed)
		return err
	}
	s.status.Set(resource.StatusHealthy)
	return nil
}

// Shutdown implements resource.Managed. It cancels every subscription.
func (s *Server) Shutdown(_ context.Context) error {
	if err := s.Close(); err != nil {
		s.status.Set(resource.StatusFailed)
		return err
	}
	s.status.Set(resource.StatusClosed)
	return nil
}

// isStarted reports whether the server is listening.
func (s *Server) isStarted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}

// Use appends middlewares to the chain applied to every handler, the first being the outermost.
func (s *Server) Use(middlewares ...MiddlewareFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.middlewares = append(s.middlewares, middlewares...)
}

// OnError sets the callback invoked when a handler returns an error.
func (s *Server) OnError(handler func(err error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errorHandler = handler
}

// HandleFunc registers handler for the messages published on subject, optionally in a queue group.
func (s *Server) HandleFunc(subject string, queue string, handler HandlerFunc) error {
	if subject == "" {
		return fmt.Errorf("failed to register a handler: %w", ErrEmptySubject)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return fmt.Errorf("failed to register a handler on %q: %w", subject, ErrServerAlreadyStarted)
	}
	s.registrations = append(s.registrations, registration{subject: subject, queue: queue, handler: handler})
	return nil
}

// ListenAndServe subscribes every registered handler without blocking. It fails when the server is already
// started; a server that failed to start holds no subscription and can be started again.
func (s *Server) ListenAndServe() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("failed to start the NATS server: %w", ErrServerAlreadyStarted)
	}

	// The requests outlive Init, so their context is canceled by Close rather than derived from a caller.
	servingCtx, cancelServing := context.WithCancel(context.Background())
	for _, reg := range s.registrations {
		subscription, err := s.nc.QueueSubscribe(reg.subject, reg.queue, s.dispatch(servingCtx, reg.handler))
		if err != nil {
			// The subscribe error is what matters; the cleanup error is dropped.
			_ = s.unsubscribeAll()
			cancelServing()
			return fmt.Errorf("failed to subscribe to subject %q: %w", reg.subject, err)
		}
		s.subscriptions = append(s.subscriptions, subscription)
	}
	s.started = true
	s.cancelServing = cancelServing
	return nil
}

// Close cancels every subscription, then the context of the requests, and returns the first error encountered.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.unsubscribeAll()
	if s.cancelServing != nil {
		s.cancelServing()
	}
	return err
}

// unsubscribeAll cancels every subscription; the caller must hold the mutex.
func (s *Server) unsubscribeAll() error {
	var firstErr error
	for _, subscription := range s.subscriptions {
		if err := subscription.Unsubscribe(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to unsubscribe: %w", err)
		}
	}
	s.subscriptions = nil
	return firstErr
}

// dispatch adapts handler to a nats.MsgHandler with the middleware chain and error callback.
// The caller must hold the mutex.
func (s *Server) dispatch(ctx context.Context, handler HandlerFunc) nats.MsgHandler {
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		handler = s.middlewares[i](handler)
	}

	return func(msg *nats.Msg) {
		request := &Request{Context: ctx, Msg: msg}
		if err := handler(request, s.nc.PublishMsg); err != nil {
			s.reportError(err)
		}
	}
}

// reportError hands err to the callback set with OnError, which may change while messages are dispatched.
func (s *Server) reportError(err error) {
	s.mu.Lock()
	errorHandler := s.errorHandler
	s.mu.Unlock()

	if errorHandler != nil {
		errorHandler(err)
	}
}

// Server implements the resource.Managed interface.
var _ resource.Managed = (*Server)(nil)
