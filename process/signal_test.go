package process_test

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/app"
	athanorfixture "github.com/grandper/go-athanor/fixture"
	"github.com/grandper/go-athanor/process"
)

// ListenToSignals is an app.ProcessFunc.
var _ app.ProcessFunc = process.ListenToSignals

func TestListenToSignals(t *testing.T) {
	ctx := context.Background()

	t.Run("should return nil when the context is canceled", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)

		result := make(chan error, 1)
		go func() { result <- process.ListenToSignals(cancelCtx) }()

		cancel()

		err := athanorfixture.Receive(t, result, 2*time.Second,
			"ListenToSignals did not return after the context was canceled")
		require.NoError(t, err)
	})
}

func TestWaitForSignal(t *testing.T) {
	ctx := context.Background()

	t.Run("should report a termination signal with ErrPOSIXSignalReceived", func(t *testing.T) {
		signalCh := make(chan os.Signal, 1)
		signalCh <- syscall.SIGTERM

		err := process.WaitForSignal(ctx, signalCh)

		require.ErrorIs(t, err, process.ErrPOSIXSignalReceived)
	})

	t.Run("should return nil when the context is canceled first", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		err := process.WaitForSignal(cancelCtx, make(chan os.Signal))

		require.NoError(t, err)
	})
}
