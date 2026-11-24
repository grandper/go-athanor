package fixture

import (
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	natsserver "github.com/grandper/go-athanor/resource/server/nats"
)

// FakeHandler is a configurable [natsserver.Handler] that records its registration.
type FakeHandler struct {
	mu           sync.Mutex
	registered   bool
	registerFunc func(server *natsserver.Server) error
	handleFuncs  []handleFunc
	registerErr  error
}

// handleFunc is a handler function Register subscribes on the server.
type handleFunc struct {
	subject string
	queue   string
	handler natsserver.HandlerFunc
}

// NewFakeHandler returns a FakeHandler that wires no subscription.
func NewFakeHandler() *FakeHandler {
	return &FakeHandler{}
}

// WithRegisterFunc makes Register delegate to fn.
func (fh *FakeHandler) WithRegisterFunc(fn func(server *natsserver.Server) error) *FakeHandler {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.registerFunc = fn
	return fh
}

// WithHandleFunc makes Register subscribe handler on subject, optionally in a queue group.
func (fh *FakeHandler) WithHandleFunc(subject string, queue string, handler natsserver.HandlerFunc) *FakeHandler {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.handleFuncs = append(fh.handleFuncs, handleFunc{subject: subject, queue: queue, handler: handler})
	return fh
}

// FailRegisterWith makes Register fail with err.
func (fh *FakeHandler) FailRegisterWith(err error) *FakeHandler {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.registerErr = err
	return fh
}

// Register implements the natsserver.Handler interface.
func (fh *FakeHandler) Register(server *natsserver.Server) error {
	fh.mu.Lock()
	registerFunc := fh.registerFunc
	registerErr := fh.registerErr
	handleFuncs := append([]handleFunc(nil), fh.handleFuncs...)
	fh.registered = true
	fh.mu.Unlock()

	if registerErr != nil {
		return registerErr
	}
	for _, hf := range handleFuncs {
		if err := server.HandleFunc(hf.subject, hf.queue, hf.handler); err != nil {
			return err
		}
	}
	if registerFunc != nil {
		return registerFunc(server)
	}
	return nil
}

// WasRegistered reports whether Register was called.
func (fh *FakeHandler) WasRegistered() bool {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	return fh.registered
}

// FakeHandler implements the natsserver.Handler interface.
var _ natsserver.Handler = (*FakeHandler)(nil)

const (
	waitTimeout  = 2 * time.Second
	pollInterval = 10 * time.Millisecond
)

// FakeHandlerFunc is a configurable [natsserver.HandlerFunc] that records the requests it handled.
// Pass its Handle method wherever a handler function is expected.
type FakeHandlerFunc struct {
	mu       sync.Mutex
	requests []*natsserver.Request
	reply    []byte
	err      error
	release  <-chan struct{}
}

// NewFakeHandlerFunc returns a FakeHandlerFunc that succeeds without replying.
func NewFakeHandlerFunc() *FakeHandlerFunc {
	return &FakeHandlerFunc{}
}

// FailWith makes Handle fail with err.
func (f *FakeHandlerFunc) FailWith(err error) *FakeHandlerFunc {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
	return f
}

// BlockUntil makes Handle block, once it recorded the request, until release is closed.
func (f *FakeHandlerFunc) BlockUntil(release <-chan struct{}) *FakeHandlerFunc {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.release = release
	return f
}

// ReplyWith makes Handle publish data on the reply subject of the request.
func (f *FakeHandlerFunc) ReplyWith(data []byte) *FakeHandlerFunc {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reply = data
	return f
}

// Handle is the natsserver.HandlerFunc. It records the request, blocks when configured to, replies when
// configured to, and returns the configured error.
func (f *FakeHandlerFunc) Handle(r *natsserver.Request, publish natsserver.PublishFunc) error {
	f.mu.Lock()
	f.requests = append(f.requests, r)
	reply, err, release := f.reply, f.err, f.release
	f.mu.Unlock()

	if release != nil {
		<-release
	}
	if err != nil {
		return err
	}
	if reply != nil {
		return publish(&nats.Msg{Subject: r.Msg.Reply, Data: reply})
	}
	return nil
}

// Requests returns a snapshot of the requests handled so far, in order.
func (f *FakeHandlerFunc) Requests() []*natsserver.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*natsserver.Request(nil), f.requests...)
}

// CallCount returns the number of requests handled so far.
func (f *FakeHandlerFunc) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// WaitForRequests fails the test unless at least count requests are handled within waitTimeout,
// then returns a snapshot of the requests handled so far.
func (f *FakeHandlerFunc) WaitForRequests(t *testing.T, count int) []*natsserver.Request {
	t.Helper()
	require.Eventually(t, func() bool { return f.CallCount() >= count }, waitTimeout, pollInterval,
		"the handler never received %d request(s)", count)
	return f.Requests()
}

// Handle is a natsserver.HandlerFunc.
var _ natsserver.HandlerFunc = (*FakeHandlerFunc)(nil).Handle

// FakePublishFunc is a configurable [natsserver.PublishFunc] that records the messages it published.
// Pass its Publish method wherever a publish function is expected.
type FakePublishFunc struct {
	mu        sync.Mutex
	published []*nats.Msg
	err       error
}

// NewFakePublishFunc returns a FakePublishFunc that succeeds.
func NewFakePublishFunc() *FakePublishFunc {
	return &FakePublishFunc{}
}

// FailWith makes Publish fail with err.
func (f *FakePublishFunc) FailWith(err error) *FakePublishFunc {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
	return f
}

// Publish is the natsserver.PublishFunc. It records msg, unless it is configured to fail.
func (f *FakePublishFunc) Publish(msg *nats.Msg) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, msg)
	return nil
}

// Published returns a snapshot of the messages published so far, in order.
func (f *FakePublishFunc) Published() []*nats.Msg {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*nats.Msg(nil), f.published...)
}

// Publish is a natsserver.PublishFunc.
var _ natsserver.PublishFunc = (*FakePublishFunc)(nil).Publish
