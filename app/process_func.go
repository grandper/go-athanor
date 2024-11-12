package app

import "context"

// ProcessFunc is a long-running process or a lifecycle hook driven by the app.
type ProcessFunc func(ctx context.Context) error
