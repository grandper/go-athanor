package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

var (
	// ErrPOSIXSignalReceived is returned when a POSIX signal (SIGINT, SIGTERM, SIGQUIT) is received.
	ErrPOSIXSignalReceived = errors.New("POSIX signal received")
)

// ListenToSignals blocks until a POSIX termination signal is received or the context is canceled.
func ListenToSignals(ctx context.Context) error {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer signal.Stop(signalCh)

	return WaitForSignal(ctx, signalCh)
}

// WaitForSignal blocks until a signal arrives on signalCh or the context is canceled.
func WaitForSignal(ctx context.Context, signalCh <-chan os.Signal) error {
	select {
	case <-ctx.Done():
		return nil
	case sig := <-signalCh:
		return fmt.Errorf("received %s: %w", sig, ErrPOSIXSignalReceived)
	}
}
