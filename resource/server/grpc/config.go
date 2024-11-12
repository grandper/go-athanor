package grpc

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
)

const (
	// BindInterfaceEnvVar is the environment variable naming the network interface the gRPC server binds to.
	BindInterfaceEnvVar = "GRPC_BIND_INTERFACE"

	// BindPortEnvVar is the environment variable naming the port the gRPC server binds to.
	BindPortEnvVar = "GRPC_BIND_PORT"

	// DefaultBindInterface is the default interface the gRPC server binds to.
	DefaultBindInterface = "0.0.0.0"

	// DefaultBindPort is the default port the gRPC server binds to.
	DefaultBindPort = 50051
)

const (
	minBindPort = 1
	maxBindPort = 65535
)

// ErrInvalidBindPort is returned when the bind port of the environment is not a number between 1 and 65535.
var ErrInvalidBindPort = errors.New("invalid bind port")

// Config holds the network configuration of the gRPC server.
type Config struct {
	// BindInterface is the network interface the server binds to, such as "0.0.0.0" or "127.0.0.1".
	BindInterface string

	// BindPort is the port the server binds to.
	BindPort int
}

// NewConfigFromEnv builds a Config from the environment variables, falling back to the defaults for the
// variables that are not set. It fails when the bind port is set to an invalid value.
func NewConfigFromEnv() (*Config, error) {
	bindPort := DefaultBindPort
	if value := os.Getenv(BindPortEnvVar); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil || port < minBindPort || port > maxBindPort {
			return nil, fmt.Errorf("failed to read %s=%q: %w", BindPortEnvVar, value, ErrInvalidBindPort)
		}
		bindPort = port
	}

	bindInterface := os.Getenv(BindInterfaceEnvVar)
	if bindInterface == "" {
		bindInterface = DefaultBindInterface
	}

	return &Config{BindInterface: bindInterface, BindPort: bindPort}, nil
}

// Address returns the listen address in the "interface:port" form.
func (config *Config) Address() string {
	return net.JoinHostPort(config.BindInterface, strconv.Itoa(config.BindPort))
}
