package resource

import "sync"

// ObservableStatus is a concurrency-safe status that notifies its observers when it changes.
type ObservableStatus struct {
	mu        sync.RWMutex
	value     Status
	nextID    int
	observers map[int]StatusObserver
}

// NewObservableStatus returns an ObservableStatus seeded with initial.
func NewObservableStatus(initial Status) *ObservableStatus {
	return &ObservableStatus{
		value:     initial,
		observers: make(map[int]StatusObserver),
	}
}

// Get returns the current status.
func (o *ObservableStatus) Get() Status {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.value
}

// Set updates the status and notifies all observers if the value changed.
func (o *ObservableStatus) Set(s Status) {
	o.mu.Lock()
	if o.value == s {
		o.mu.Unlock()
		return
	}
	o.value = s
	observers := make([]StatusObserver, 0, len(o.observers))
	for _, obs := range o.observers {
		observers = append(observers, obs)
	}
	o.mu.Unlock()

	for _, obs := range observers {
		obs.StatusChangedTo(s)
	}
}

// Observe registers obs for future changes and returns a function that removes it.
func (o *ObservableStatus) Observe(obs StatusObserver) func() {
	o.mu.Lock()
	id := o.nextID
	o.nextID++
	o.observers[id] = obs
	o.mu.Unlock()

	return func() {
		o.mu.Lock()
		delete(o.observers, id)
		o.mu.Unlock()
	}
}
