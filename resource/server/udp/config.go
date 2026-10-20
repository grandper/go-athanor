package udp

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	// AddressEnvVar is the environment variable naming the "host:port" address the UDP server listens on.
	AddressEnvVar = "UDP_ADDRESS"

	// MulticastEnvVar is the environment variable telling whether the address is a multicast group to join.
	MulticastEnvVar = "UDP_MULTICAST"

	// InterfaceNameEnvVar is the environment variable naming the interface the multicast group is joined on.
	InterfaceNameEnvVar = "UDP_INTERFACE_NAME"

	// BufferSizeEnvVar is the environment variable holding the size in bytes of the buffer of a datagram.
	BufferSizeEnvVar = "UDP_BUFFER_SIZE"

	// ReceivedTimeoutEnvVar is the environment variable holding the longest silence the UDP server tolerates.
	ReceivedTimeoutEnvVar = "UDP_RECEIVED_TIMEOUT"

	// DefaultMulticast is the default of the multicast flag: the address is a multicast group to join.
	DefaultMulticast = true

	// DefaultBufferSize is the default size in bytes of the buffer of a datagram, the usual Ethernet MTU.
	DefaultBufferSize = 1500

	// DefaultReceivedTimeout is the default longest silence the server tolerates.
	DefaultReceivedTimeout = 5 * time.Second
)

var (
	// ErrInvalidMulticast is returned when the multicast flag of the environment is not a boolean.
	ErrInvalidMulticast = errors.New("invalid multicast flag")

	// ErrInvalidBufferSize is returned when the buffer size is not a positive number.
	ErrInvalidBufferSize = errors.New("invalid buffer size")

	// ErrInvalidReceivedTimeout is returned when the received timeout is not a duration, or is negative.
	ErrInvalidReceivedTimeout = errors.New("invalid received timeout")
)

// Config holds the network configuration of the UDP server.
type Config struct {
	// Address is the "host:port" the server listens on: the multicast group to join when Multicast is set,
	// the local address to bind otherwise.
	Address string

	// Multicast tells whether Address is a multicast group to join on InterfaceName.
	Multicast bool

	// InterfaceName is the network interface the multicast group is joined on, such as "eth0".
	InterfaceName string

	// BufferSize is the size in bytes of the buffer allocated for each datagram; longer datagrams are truncated.
	BufferSize int

	// ReceivedTimeout is the longest silence the server tolerates before failing; zero disables the watchdog.
	ReceivedTimeout time.Duration
}

// NewConfigFromEnv builds a Config from the environment variables, falling back to the defaults for the
// variables that are not set. It fails when the multicast flag, the buffer size, or the received timeout is
// set to an invalid value.
func NewConfigFromEnv() (*Config, error) {
	multicast := DefaultMulticast
	if value := os.Getenv(MulticastEnvVar); value != "" {
		flag, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s=%q: %w", MulticastEnvVar, value, ErrInvalidMulticast)
		}
		multicast = flag
	}

	bufferSize := DefaultBufferSize
	if value := os.Getenv(BufferSizeEnvVar); value != "" {
		size, err := strconv.Atoi(value)
		if err != nil || size <= 0 {
			return nil, fmt.Errorf("failed to read %s=%q: %w", BufferSizeEnvVar, value, ErrInvalidBufferSize)
		}
		bufferSize = size
	}

	receivedTimeout := DefaultReceivedTimeout
	if value := os.Getenv(ReceivedTimeoutEnvVar); value != "" {
		timeout, err := time.ParseDuration(value)
		if err != nil || timeout < 0 {
			return nil, fmt.Errorf("failed to read %s=%q: %w", ReceivedTimeoutEnvVar, value, ErrInvalidReceivedTimeout)
		}
		receivedTimeout = timeout
	}

	return &Config{
		Address:         os.Getenv(AddressEnvVar),
		Multicast:       multicast,
		InterfaceName:   os.Getenv(InterfaceNameEnvVar),
		BufferSize:      bufferSize,
		ReceivedTimeout: receivedTimeout,
	}, nil
}
