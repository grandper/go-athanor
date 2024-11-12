package fixture_test

import (
	"fmt"
	"net"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/fixture"
)

func TestGetFreePort(t *testing.T) {
	t.Run("should return a port that can be bound", func(t *testing.T) {
		port := fixture.GetFreePort(t)
		require.Positive(t, port)

		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		require.NoError(t, err)
		require.NoError(t, listener.Close())
	})
}

func TestGetFreePortString(t *testing.T) {
	t.Run("should return a numeric port", func(t *testing.T) {
		port, err := strconv.Atoi(fixture.GetFreePortString(t))

		require.NoError(t, err)
		assert.Positive(t, port)
	})
}

func TestOnFreePort(t *testing.T) {
	t.Run("should call use with a port that can be bound", func(t *testing.T) {
		err := fixture.OnFreePort(t, func(port int) error {
			listener, errListen := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
			if errListen != nil {
				return errListen
			}
			return listener.Close()
		})

		require.NoError(t, err)
	})

	t.Run("should retry with another port when the address is already in use", func(t *testing.T) {
		var ports []int

		err := fixture.OnFreePort(t, func(port int) error {
			ports = append(ports, port)
			if len(ports) == 1 {
				return fmt.Errorf("failed to listen: %w", syscall.EADDRINUSE)
			}
			return nil
		})

		require.NoError(t, err)
		assert.Len(t, ports, 2)
	})

	t.Run("fails without retrying when use fails for another reason", func(t *testing.T) {
		calls := 0

		err := fixture.OnFreePort(t, func(int) error {
			calls++
			return assert.AnError
		})

		require.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, 1, calls)
	})

	t.Run("fails when every attempt finds its address already in use", func(t *testing.T) {
		calls := 0

		err := fixture.OnFreePort(t, func(int) error {
			calls++
			return syscall.EADDRINUSE
		})

		require.ErrorIs(t, err, syscall.EADDRINUSE)
		assert.Equal(t, fixture.FreePortAttempts, calls)
	})
}

func TestRunOnFreePort(t *testing.T) {
	// listenUntil occupies the port until release is closed, as a server running until its context is canceled.
	listenUntil := func(port int, release <-chan struct{}) error {
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			return err
		}
		<-release
		return listener.Close()
	}

	t.Run("should return the port once run accepts connections, and the result of run", func(t *testing.T) {
		release := make(chan struct{})

		port, result := fixture.RunOnFreePort(t, func(port int) error { return listenUntil(port, release) })

		assert.True(t, fixture.CanConnect(net.JoinHostPort("127.0.0.1", strconv.Itoa(port))))
		close(release)
		require.NoError(t, fixture.Receive(t, result, 5*time.Second, "run never returned"))
	})

	t.Run("should run again on another port when the address is already in use", func(t *testing.T) {
		release := make(chan struct{})
		ports := make(chan int, fixture.FreePortAttempts)

		port, result := fixture.RunOnFreePort(t, func(port int) error {
			ports <- port
			if len(ports) == 1 {
				return fmt.Errorf("failed to listen: %w", syscall.EADDRINUSE)
			}
			return listenUntil(port, release)
		})

		close(release)
		require.NoError(t, fixture.Receive(t, result, 5*time.Second, "run never returned"))
		require.Len(t, ports, 2)
		<-ports
		assert.Equal(t, <-ports, port, "the port must be the one of the run that succeeded")
	})

	t.Run("should hand over the error of a run that fails for another reason", func(t *testing.T) {
		calls := make(chan struct{}, fixture.FreePortAttempts)

		_, result := fixture.RunOnFreePort(t, func(int) error {
			calls <- struct{}{}
			return assert.AnError
		})

		require.ErrorIs(t, fixture.Receive(t, result, 5*time.Second, "run never returned"), assert.AnError)
		assert.Len(t, calls, 1)
	})

	t.Run("should hand over the error when every run finds its address already in use", func(t *testing.T) {
		calls := make(chan struct{}, fixture.FreePortAttempts)

		_, result := fixture.RunOnFreePort(t, func(int) error {
			calls <- struct{}{}
			return syscall.EADDRINUSE
		})

		require.ErrorIs(t, fixture.Receive(t, result, 5*time.Second, "run never returned"), syscall.EADDRINUSE)
		assert.Len(t, calls, fixture.FreePortAttempts)
	})
}

func TestCanConnect(t *testing.T) {
	t.Run("should report whether a server listens on the address", func(t *testing.T) {
		address := net.JoinHostPort("127.0.0.1", fixture.GetFreePortString(t))
		assert.False(t, fixture.CanConnect(address))

		listener := fixture.ListenOn(t, address)
		defer func() { require.NoError(t, listener.Close()) }()

		assert.True(t, fixture.CanConnect(address))
	})
}

func TestListenOn(t *testing.T) {
	t.Run("should occupy the given address", func(t *testing.T) {
		address := net.JoinHostPort("127.0.0.1", fixture.GetFreePortString(t))

		listener := fixture.ListenOn(t, address)
		defer func() { require.NoError(t, listener.Close()) }()

		_, err := net.Listen("tcp", address)
		assert.Error(t, err, "the address must already be in use")
	})
}

func TestListenOnFreePort(t *testing.T) {
	t.Run("should occupy a port until the test ends", func(t *testing.T) {
		var address string
		t.Run("test holding the listener", func(t *testing.T) {
			listener := fixture.ListenOnFreePort(t)
			address = listener.Addr().String()

			assert.True(t, fixture.CanConnect(address))
		})

		assert.False(t, fixture.CanConnect(address), "the listener must be closed once its test ends")
	})

	t.Run("should tolerate a listener closed by the code under test", func(t *testing.T) {
		listener := fixture.ListenOnFreePort(t)

		require.NoError(t, listener.Close())
	})
}

func TestPortOf(t *testing.T) {
	t.Run("should return the port the listener is bound to", func(t *testing.T) {
		listener := fixture.ListenOnFreePort(t)

		port := fixture.PortOf(t, listener)

		assert.Equal(t, listener.Addr().String(), net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	})
}
