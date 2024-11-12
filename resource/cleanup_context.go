package resource

import (
	"context"
	"time"
)

// CleanupTimeout bounds the cleanups run with a CleanupContext.
const CleanupTimeout = 30 * time.Second

// CleanupContext derives the context of a cleanup, such as a shutdown or a rollback, from ctx. It keeps the
// values of ctx but not its cancellation, which is often the very reason for the cleanup, and it expires
// after CleanupTimeout so a cleanup that hangs cannot block forever.
func CleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), CleanupTimeout)
}
