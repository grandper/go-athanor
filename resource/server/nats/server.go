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

	mu             sync.Mutex
	middlewares    []MiddlewareFunc
	registrations  []registration
	subscriptions  []*nats.Subscription
	hooks          *hookSet // frozen once started, so the hooks run without the mutex
	started        bool
	cancelServing  context.CancelFunc // cancels the context of the requests
	resetCallbacks []func()           // resets the connection callbacks the server installed

	// hooksRunning tracks the subscription hooks still to run: the closed handler and the status goroutine of
	// every subscription.
	hooksRunning sync.WaitGroup
}

// NewServer returns a Server consuming messages from the NATS connection nc and dispatching to the handlers.
func NewServer(nc *nats.Conn, handlers ...Handler) *Server {
	return &Server{
		name:     DefaultName,
		nc:       nc,
		handlers: handlers,
		status:   resource.NewObservableStatus(resource.StatusInitializing),
		hooks:    &hookSet{},
	}
}

// WithName overrides the name identifying the server inside a resource.List.
func (s *Server) WithName(name string) *Server {
	s.name = name
	return s
}

// WithHooks adds hooks watching the lifecycle of the server. You can call it several times: every hook is kept,
// and the hooks watching the same event run in the order they were given. The hooks are installed when the server
// starts, so hooks added after the server started are ignored.
//
// While the server runs, it owns the connection callbacks of the events its hooks watch: an OnConnectionLost,
// OnConnectionRestored or OnConnectionClosed hook replaces the disconnect, reconnect or closed callback given to
// nats.Connect, which is neither chained nor restored, and Shutdown resets that callback to nil. The callbacks of
// the events no hook watches are left untouched. Give your connection callbacks through WithHooks rather than
// nats.Connect.
func (s *Server) WithHooks(hooks ...Hook) *Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		s.hooks.add(hooks...)
	}
	return s
}

// Name implements resource.Managed.
func (s *Server) Name() string { return s.name }

// Status implements resource.Managed.
func (s *Server) Status() *resource.ObservableStatus { return s.status }

// Init implements resource.Managed. It registers the handlers and starts listening.
// A server starts once: Init fails with ErrServerAlreadyStarted on a running server and on a server that was
// shut down, while an Init that failed can be retried.
//
// When a subscription fails to open, Init waits for the OnSubscriptionClosed hooks of the subscriptions already
// opened, or for ctx to be done, before it returns the error.
func (s *Server) Init(ctx context.Context) error {
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
		// The subscribe error is what matters, whether or not the hooks ran in time.
		errWait := s.waitForHooks(ctx)
		s.status.Set(resource.StatusFailed)
		return errors.Join(err, errWait)
	}
	s.status.Set(resource.StatusHealthy)
	return nil
}

// Shutdown implements resource.Managed. It cancels every subscription, waits for the subscription hooks to run,
// then resets to nil the connection callbacks the server installed for its hooks. It fails when a subscription
// fails to unsubscribe, or when ctx is done before the hooks ran; the connection callbacks are reset either way.
func (s *Server) Shutdown(ctx context.Context) error {
	err := s.Close()
	if waitErr := s.waitForHooks(ctx); err == nil {
		err = waitErr
	}
	s.resetConnectionCallbacks()
	if err != nil {
		s.status.Set(resource.StatusFailed)
		return err
	}
	s.status.Set(resource.StatusClosed)
	return nil
}

// waitForHooks waits for the subscription hooks still to run, or for ctx to be done.
func (s *Server) waitForHooks(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.hooksRunning.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("failed to wait for the NATS hooks: %w", ctx.Err())
	}
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

// ListenAndServe subscribes every registered handler without blocking, then installs the connection hooks and
// reports the connection established. It fails when the server is already started; a server that failed to start
// holds no subscription, installed no hook, and can be started again.
func (s *Server) ListenAndServe() error {
	if err := s.start(); err != nil {
		return err
	}
	// The hooks are frozen once started, and run without the mutex so that they may use the server.
	if s.nc.IsConnected() {
		s.hooks.notifyConnectionEstablished(s.nc)
	}
	return nil
}

// start subscribes every registered handler, installs the connection callbacks of the hooks, and marks the server
// started, under the mutex. ListenAndServe calls the hooks reporting the start once the mutex is released.
func (s *Server) start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("failed to start the NATS server: %w", ErrServerAlreadyStarted)
	}

	// The requests outlive Init, so their context is canceled by Close rather than derived from a caller.
	servingCtx, cancelServing := context.WithCancel(context.Background())
	closures := make([]<-chan struct{}, 0, len(s.registrations))
	for _, reg := range s.registrations {
		subscription, err := s.nc.QueueSubscribe(reg.subject, reg.queue, s.dispatch(servingCtx, reg.handler))
		if err != nil {
			// The subscribe error is what matters; the cleanup error is dropped.
			_ = s.unsubscribeAll()
			cancelServing()
			return fmt.Errorf("failed to subscribe to subject %q: %w", reg.subject, err)
		}
		closures = append(closures, s.watchClosure(subscription))
		s.subscriptions = append(s.subscriptions, subscription)
		s.hooks.notifySubscriptionOpened(s.nc, subscription)
	}
	if s.hooks.watchesSubscriptionStatus() {
		for i, subscription := range s.subscriptions {
			s.watchStatus(subscription, closures[i])
		}
	}
	s.installConnectionCallbacks()
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

// watchClosure installs on subscription a closed handler, tracked by hooksRunning, that closes the returned channel
// and runs the OnSubscriptionClosed hooks. The status goroutine needs that channel too, so the handler is installed
// when a hook watches either the closure or the status; otherwise watchClosure installs nothing and returns nil.
func (s *Server) watchClosure(subscription *nats.Subscription) <-chan struct{} {
	handler := s.hooks.subscriptionClosedHandler(s.nc, subscription)
	if handler == nil && !s.hooks.watchesSubscriptionStatus() {
		return nil
	}
	closed := make(chan struct{})
	s.hooksRunning.Add(1)
	subscription.SetClosedHandler(func(subject string) {
		defer s.hooksRunning.Done()
		close(closed)
		if handler != nil {
			handler(subject)
		}
	})
	return closed
}

// watchStatus starts the goroutine reporting the status changes of subscription to the OnSubscriptionStatusChanged
// hooks, tracked by hooksRunning. The goroutine ends when the client closes the channel of the statuses, or when
// the subscription closed; the client never reports nats.SubscriptionClosed when the connection closes, so the
// goroutine reports it then. Repeated statuses are dropped, so Closed is reported once even when the client sent it.
func (s *Server) watchStatus(subscription *nats.Subscription, closed <-chan struct{}) {
	statuses := subscription.StatusChanged()
	s.hooksRunning.Add(1)
	go func() {
		defer s.hooksRunning.Done()
		last := nats.SubscriptionActive
		change := func(status nats.SubStatus) {
			if status != last {
				last = status
				s.hooks.notifySubscriptionStatusChanged(s.nc, subscription, status)
			}
		}
		defer func() { change(nats.SubscriptionClosed) }()

		for {
			select {
			case status, ok := <-statuses:
				if !ok {
					return
				}
				change(status)
			case <-closed:
				// Report the statuses the client already sent, without waiting for more.
				for len(statuses) > 0 {
					change(<-statuses)
				}
				return
			}
		}
	}()
}

// installConnectionCallbacks installs the connection callbacks the hooks watch, and remembers how to reset them.
// The caller must hold the mutex.
func (s *Server) installConnectionCallbacks() {
	if handler := s.hooks.disconnectErrHandler(); handler != nil {
		s.nc.SetDisconnectErrHandler(handler)
		s.resetCallbacks = append(s.resetCallbacks, func() { s.nc.SetDisconnectErrHandler(nil) })
	}
	if handler := s.hooks.reconnectHandler(); handler != nil {
		s.nc.SetReconnectHandler(handler)
		s.resetCallbacks = append(s.resetCallbacks, func() { s.nc.SetReconnectHandler(nil) })
	}
	if handler := s.hooks.closedHandler(); handler != nil {
		s.nc.SetClosedHandler(handler)
		s.resetCallbacks = append(s.resetCallbacks, func() { s.nc.SetClosedHandler(nil) })
	}
}

// resetConnectionCallbacks resets to nil the connection callbacks the server installed.
func (s *Server) resetConnectionCallbacks() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, reset := range s.resetCallbacks {
		reset()
	}
	s.resetCallbacks = nil
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

// dispatch adapts handler to a nats.MsgHandler with the middleware chain and the OnHandlerFailed hooks.
// The caller must hold the mutex.
func (s *Server) dispatch(ctx context.Context, handler HandlerFunc) nats.MsgHandler {
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		handler = s.middlewares[i](handler)
	}

	return func(msg *nats.Msg) {
		request := &Request{Context: ctx, Msg: msg}
		if err := handler(request, s.nc.PublishMsg); err != nil {
			s.hooks.notifyHandlerFailed(s.nc, msg, err)
		}
	}
}

// Server implements the resource.Managed interface.
var _ resource.Managed = (*Server)(nil)
