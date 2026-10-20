package udp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/grandper/go-athanor/resource"
)

// DefaultName is the default name of the UDP server resource.
const DefaultName = "udp-server"

var (
	// ErrNilConfig is returned when the configuration is nil.
	ErrNilConfig = errors.New("config is nil")

	// ErrNilHandler is returned when the handler is nil.
	ErrNilHandler = errors.New("handler is nil")

	// ErrEmptyAddress is returned when the address is empty.
	ErrEmptyAddress = errors.New("address is empty")

	// ErrEmptyInterfaceName is returned when multicast is enabled and the interface name is empty.
	ErrEmptyInterfaceName = errors.New("interface name is empty")

	// ErrReceivedTimeout is the failure of a server that received no datagram within the received timeout.
	ErrReceivedTimeout = errors.New("received timeout")

	// ErrServerAlreadyStarted is returned when Init is called on a server that already started,
	// whether it is still running or was shut down.
	ErrServerAlreadyStarted = errors.New("UDP server already started")
)

// Server is a managed resource reading the datagrams of a UDP socket, plain or multicast, and handing them
// over to its handler one at a time.
type Server struct {
	name       string
	config     *Config
	handler    Handler
	connection Connection // set with WithConnection, nil to bind the address of the configuration
	status     *resource.ObservableStatus

	mu            sync.Mutex
	started       bool // claimed by the first successful Init, for good
	errorHandler  func(err error)
	active        Connection         // the socket being read, set by a successful Init
	cancelServing context.CancelFunc // cancels the context of the handler
	stopped       bool               // the socket was closed, for good
	done          chan struct{}      // closed when the read loop exits
	serveErr      error              // failure of the read loop, read by Shutdown
	closeErr      error              // failure of the socket close, read by Shutdown
}

// NewServer creates a UDP server resource handing the datagrams received on the configured address over to
// the handler.
func NewServer(config *Config, handler Handler) *Server {
	return &Server{
		name:    DefaultName,
		config:  config,
		handler: handler,
		status:  resource.NewObservableStatus(resource.StatusInitializing),
	}
}

// WithName overrides the name identifying the server inside a resource.List.
func (s *Server) WithName(name string) *Server {
	s.name = name
	return s
}

// WithConnection makes the server read connection instead of binding the address of its configuration, so you
// can open the socket with the options you need. The server owns connection from then on and closes it when it
// stops. Only the buffer size and the received timeout of the configuration are used.
func (s *Server) WithConnection(connection Connection) *Server {
	s.connection = connection
	return s
}

// Name implements resource.Managed.
func (s *Server) Name() string { return s.name }

// Status implements resource.Managed.
func (s *Server) Status() *resource.ObservableStatus { return s.status }

// LocalAddress returns the local address of the socket, or nil while the server is not serving.
func (s *Server) LocalAddress() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil {
		return nil
	}
	return s.active.LocalAddr()
}

// OnError sets the callback invoked when the handler returns an error. It can be set at any time.
func (s *Server) OnError(handler func(err error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errorHandler = handler
}

// Init implements resource.Managed. It opens the socket and reads it in the background.
// A server starts once: Init fails with ErrServerAlreadyStarted on a running server and on a server that was
// shut down, while an Init that failed can be retried.
func (s *Server) Init(ctx context.Context) error {
	if !s.claimStart() {
		// The running server is left untouched: a second socket and read loop would leak.
		return fmt.Errorf("failed to init the UDP server: %w", ErrServerAlreadyStarted)
	}
	if err := s.start(ctx); err != nil {
		s.releaseStart()
		s.status.Set(resource.StatusFailed)
		return err
	}
	return nil
}

// start validates the configuration, opens the socket, and reads it in the background.
func (s *Server) start(ctx context.Context) error {
	if err := s.validate(); err != nil {
		return fmt.Errorf("failed to init the UDP server: %w", err)
	}

	// Open synchronously so address errors are returned by Init.
	conn, err := s.listen()
	if err != nil {
		return err
	}

	// The handler outlives Init: its context keeps the values of ctx and is canceled by Shutdown instead.
	servingCtx, cancelServing := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})
	s.mu.Lock()
	s.active = conn
	s.cancelServing = cancelServing
	s.done = done
	s.mu.Unlock()

	s.status.Set(resource.StatusHealthy)

	go func() {
		defer close(done)
		s.serve(servingCtx, conn)
	}()
	return nil
}

// validate checks the configuration and the handler. A socket of your own needs no address to bind, so only
// the fields driving the read loop are checked then.
func (s *Server) validate() error {
	if s.config == nil {
		return ErrNilConfig
	}
	if s.handler == nil {
		return ErrNilHandler
	}
	if s.connection == nil {
		if s.config.Address == "" {
			return ErrEmptyAddress
		}
		if s.config.Multicast && s.config.InterfaceName == "" {
			return ErrEmptyInterfaceName
		}
	}
	if s.config.BufferSize <= 0 {
		return fmt.Errorf("%w: %d", ErrInvalidBufferSize, s.config.BufferSize)
	}
	if s.config.ReceivedTimeout < 0 {
		return fmt.Errorf("%w: %s", ErrInvalidReceivedTimeout, s.config.ReceivedTimeout)
	}
	return nil
}

// listen returns the socket set with WithConnection, or binds the address of the configuration, joining the
// multicast group when one is configured.
func (s *Server) listen() (Connection, error) {
	if s.connection != nil {
		return s.connection, nil
	}

	address, err := net.ResolveUDPAddr("udp4", s.config.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %q: %w", s.config.Address, err)
	}
	if !s.config.Multicast {
		conn, errListen := net.ListenUDP("udp4", address)
		if errListen != nil {
			return nil, fmt.Errorf("failed to listen on %q: %w", s.config.Address, errListen)
		}
		return conn, nil
	}

	iface, err := net.InterfaceByName(s.config.InterfaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to find the interface %q: %w", s.config.InterfaceName, err)
	}
	conn, err := net.ListenMulticastUDP("udp4", iface, address)
	if err != nil {
		return nil, fmt.Errorf("failed to join the multicast group %q on %q: %w",
			s.config.Address, s.config.InterfaceName, err)
	}
	return conn, nil
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

// serve reads the socket until it is closed, handing each datagram over to the handler. A read that fails,
// or that waits longer than the received timeout, fails the server, unless the server is stopping.
func (s *Server) serve(ctx context.Context, conn Connection) {
	for {
		buffer := make([]byte, s.config.BufferSize)
		n, source, err := s.read(conn, buffer)
		if err != nil {
			if ctx.Err() == nil {
				s.fail(err)
			}
			return
		}

		datagram := Datagram{Payload: buffer[:n], Source: source}
		if errHandle := s.handler.Handle(ctx, datagram); errHandle != nil {
			s.reportError(errHandle)
		}
	}
}

// read reads one datagram into buffer, waiting for it no longer than the received timeout when there is one:
// the silence of the source is a deadline on the read, as the read timeout of the HTTP server is.
func (s *Server) read(conn Connection, buffer []byte) (int, netip.AddrPort, error) {
	timeout := s.config.ReceivedTimeout
	if timeout > 0 {
		if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return 0, netip.AddrPort{}, fmt.Errorf("failed to set the read deadline: %w", err)
		}
	}

	n, source, err := conn.ReadFromUDPAddrPort(buffer)
	switch {
	case err == nil:
		return n, source, nil
	case errors.Is(err, os.ErrDeadlineExceeded):
		return 0, netip.AddrPort{}, fmt.Errorf("no data received since %s: %w", timeout, ErrReceivedTimeout)
	default:
		return 0, netip.AddrPort{}, fmt.Errorf("failed to read from UDP: %w", err)
	}
}

// fail records the failure, stops serving, and reports StatusFailed.
func (s *Server) fail(err error) {
	s.mu.Lock()
	s.serveErr = err
	s.mu.Unlock()

	s.stopServing()
	s.status.Set(resource.StatusFailed)
}

// stopServing cancels the serving context, then closes the socket, which unblocks the read loop. It does
// so once: the later calls change nothing.
func (s *Server) stopServing() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	// Canceled before the close, so the read loop takes the close for the graceful stop it is.
	s.cancelServing()
	s.closeErr = s.active.Close()
}

// reportError hands err to the callback set with OnError, which may change while datagrams are handled.
func (s *Server) reportError(err error) {
	s.mu.Lock()
	errorHandler := s.errorHandler
	s.mu.Unlock()

	if errorHandler != nil {
		errorHandler(err)
	}
}

// Shutdown implements resource.Managed. It closes the socket and waits for the handler to return, or for
// ctx to expire. Shutting down a server that failed returns its failure.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()

	if done == nil {
		// Init never succeeded: nothing to stop.
		s.status.Set(resource.StatusClosed)
		return nil
	}

	s.stopServing()
	select {
	case <-done:
	case <-ctx.Done():
		s.status.Set(resource.StatusFailed)
		return fmt.Errorf("failed to shut down the UDP server: %w", ctx.Err())
	}

	s.mu.Lock()
	serveErr, closeErr := s.serveErr, s.closeErr
	s.mu.Unlock()
	switch {
	case serveErr != nil:
		s.status.Set(resource.StatusFailed)
		return fmt.Errorf("failed to serve UDP: %w", serveErr)
	case closeErr != nil:
		s.status.Set(resource.StatusFailed)
		return fmt.Errorf("failed to close the UDP socket: %w", closeErr)
	}
	s.status.Set(resource.StatusClosed)
	return nil
}

// Server implements the resource.Managed interface.
var _ resource.Managed = (*Server)(nil)
