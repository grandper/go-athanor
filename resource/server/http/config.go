package http

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
)

const (
	// BindInterfaceEnvVar is the environment variable naming the network interface the HTTP server binds to.
	BindInterfaceEnvVar = "HTTP_BIND_INTERFACE"

	// BindPortEnvVar is the environment variable naming the port the HTTP server binds to.
	BindPortEnvVar = "HTTP_BIND_PORT"

	// TLSCertFileEnvVar is the environment variable naming the path to the TLS certificate file.
	TLSCertFileEnvVar = "HTTP_TLS_CERT_FILE"

	// TLSKeyFileEnvVar is the environment variable naming the path to the TLS private key file.
	TLSKeyFileEnvVar = "HTTP_TLS_KEY_FILE"

	// DefaultBindInterface is the default interface the HTTP server binds to.
	DefaultBindInterface = "0.0.0.0"

	// DefaultBindPort is the default port the HTTP server binds to.
	DefaultBindPort = 8080
)

const (
	minBindPort = 1
	maxBindPort = 65535
)

// ErrInvalidBindPort is returned when the bind port of the environment is not a number between 1 and 65535.
var ErrInvalidBindPort = errors.New("invalid bind port")

// Config holds the network and TLS configuration of the HTTP server.
type Config struct {
	// BindInterface is the network interface the server binds to, such as "0.0.0.0" or "127.0.0.1".
	BindInterface string

	// BindPort is the port the server binds to.
	BindPort int

	// TLSCertFile is the path to the TLS certificate file. TLS is served when both TLS files are set.
	TLSCertFile string

	// TLSKeyFile is the path to the TLS private key file. TLS is served when both TLS files are set.
	TLSKeyFile string
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

	return &Config{
		BindInterface: bindInterface,
		BindPort:      bindPort,
		TLSCertFile:   os.Getenv(TLSCertFileEnvVar),
		TLSKeyFile:    os.Getenv(TLSKeyFileEnvVar),
	}, nil
}

// Address returns the listen address in the "interface:port" form.
func (config *Config) Address() string {
	return net.JoinHostPort(config.BindInterface, strconv.Itoa(config.BindPort))
}

// TLSIsAvailable reports whether both the TLS certificate and key files are configured.
func (config *Config) TLSIsAvailable() bool {
	return config.TLSCertFile != "" && config.TLSKeyFile != ""
}
