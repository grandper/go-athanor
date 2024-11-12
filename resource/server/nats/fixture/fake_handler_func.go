package fixture

import (
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	natsserver "github.com/grandper/go-athanor/resource/server/nats"
)

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

// ReplyWith makes Handle publish data on the reply subject of the request.
func (f *FakeHandlerFunc) ReplyWith(data []byte) *FakeHandlerFunc {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reply = data
	return f
}

// Handle is the natsserver.HandlerFunc. It records the request, replies when configured to, and returns
// the configured error.
func (f *FakeHandlerFunc) Handle(r *natsserver.Request, publish natsserver.PublishFunc) error {
	f.mu.Lock()
	f.requests = append(f.requests, r)
	reply, err := f.reply, f.err
	f.mu.Unlock()

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
