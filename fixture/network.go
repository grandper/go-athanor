// Package fixture provides reusable testing objects shared by the packages of the repository.
package fixture

import (
	"context"
	"errors"
	"net"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// FreePortAttempts is the number of ports OnFreePort and RunOnFreePort try before giving up.
const FreePortAttempts = 5

const (
	bindTimeout      = 5 * time.Second
	bindPollInterval = 5 * time.Millisecond
)

// GetFreePort returns a TCP port that was free a moment ago. The probing listener is closed before the port
// is returned, so another process can take it meanwhile: prefer ListenOnFreePort whenever the code under
// test can be handed a listener, or only needs an address that is already taken, and OnFreePort or
// RunOnFreePort whenever it binds the port itself.
func GetFreePort(t *testing.T) int {
	t.Helper()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { require.NoError(t, listener.Close()) }()

	return PortOf(t, listener)
}

// GetFreePortString returns a free TCP port as a string.
func GetFreePortString(t *testing.T) string {
	t.Helper()
	return strconv.Itoa(GetFreePort(t))
}

// OnFreePort calls use with a free TCP port. Another process can take the port before use binds it, so
// when use fails because the address is already in use, OnFreePort calls it again with another port, up to
// FreePortAttempts times. It returns the error of the last call.
func OnFreePort(t *testing.T, use func(port int) error) error {
	t.Helper()

	var err error
	for attempt := 0; attempt < FreePortAttempts; attempt++ {
		err = use(GetFreePort(t))
		if !errors.Is(err, syscall.EADDRINUSE) {
			return err
		}
	}
	return err
}

// RunOnFreePort is OnFreePort for a run that blocks while it serves, such as app.Run: it calls run in the
// background with a free TCP port, and returns that port once something accepts connections on it, with the
// channel receiving the result of run. When run fails because the address is already in use, RunOnFreePort
// calls it again with another port, up to FreePortAttempts times; any other result, and the failure of the
// last attempt, is handed over through the channel. Build everything run needs inside it: it may be called
// more than once.
func RunOnFreePort(t *testing.T, run func(port int) error) (int, <-chan error) {
	t.Helper()

	var port int
	var result chan error
	for attempt := 0; attempt < FreePortAttempts; attempt++ {
		port = GetFreePort(t)
		result = make(chan error, 1)
		go func(port int, result chan<- error) { result <- run(port) }(port, result)

		returned, err := waitForBinding(port, result)
		if !returned {
			return port, result
		}
		// The result was consumed: hand it back to the caller, or to the next attempt.
		result <- err
		if !errors.Is(err, syscall.EADDRINUSE) {
			return port, result
		}
	}
	return port, result
}

// waitForBinding waits until something accepts connections on the port, or until run returned. It reports
// whether run returned, and its result in that case.
func waitForBinding(port int, result <-chan error) (bool, error) {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	ticker := time.NewTicker(bindPollInterval)
	defer ticker.Stop()
	timeout := time.After(bindTimeout)

	for {
		select {
		case err := <-result:
			return true, err
		case <-timeout:
			// Nothing was bound: the assertions of the caller tell the story better.
			return false, nil
		case <-ticker.C:
			if CanConnect(address) {
				return false, nil
			}
		}
	}
}

// ListenOn occupies the given TCP address and returns the listener.
func ListenOn(t *testing.T, address string) net.Listener {
	t.Helper()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(context.Background(), "tcp", address)
	require.NoError(t, err)
	return listener
}

// ListenOnFreePort occupies a loopback TCP port picked by the operating system and returns the listener,
// closed when the test ends. Unlike GetFreePort, the port cannot be taken by anyone else meanwhile.
func ListenOnFreePort(t *testing.T) net.Listener {
	t.Helper()

	listener := ListenOn(t, "127.0.0.1:0")
	// The code under test may own the listener and close it first.
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

// PortOf returns the TCP port the listener is bound to.
func PortOf(t *testing.T, listener net.Listener) int {
	t.Helper()

	address, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok, "the listener is not bound to a TCP address")
	return address.Port
}

// CanConnect reports whether a TCP connection to the given address succeeds.
func CanConnect(address string) bool {
	dialer := net.Dialer{Timeout: time.Second}
	conn, err := dialer.DialContext(context.Background(), "tcp", address)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
