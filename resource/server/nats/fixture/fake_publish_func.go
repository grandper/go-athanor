package fixture

import (
	"sync"

	"github.com/nats-io/nats.go"

	natsserver "github.com/grandper/go-athanor/resource/server/nats"
)

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
