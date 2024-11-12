package resource

// StatusObserver is notified whenever an observed status changes.
type StatusObserver interface {
	// StatusChangedTo is called synchronously with the new status after each change.
	StatusChangedTo(newStatus Status)
}

// StatusObserverFunc adapts an ordinary function to a StatusObserver.
type StatusObserverFunc func(Status)

// StatusChangedTo implements the StatusObserver interface.
func (f StatusObserverFunc) StatusChangedTo(newStatus Status) { f(newStatus) }

// StatusObserverFunc implements the StatusObserver interface.
var _ StatusObserver = (StatusObserverFunc)(nil)
