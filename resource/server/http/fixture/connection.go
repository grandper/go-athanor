package fixture

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// OpenHijackedConnection sends a GET request on path to the HTTP server at address, on a raw TCP connection,
// checks that the server takes the connection over with 101 Switching Protocols, and returns the connection,
// closed when the test ends.
func OpenHijackedConnection(t *testing.T, address string, path string) net.Conn {
	t.Helper()

	var dialer net.Dialer
	conn, err := dialer.DialContext(context.Background(), "tcp", address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	_, err = fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\n\r\n", path, address)
	require.NoError(t, err)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(respondsWithin)))
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)
	require.NoError(t, conn.SetReadDeadline(time.Time{}))
	return conn
}

// GetWithoutKeepAlive sends a GET request to url on a connection of its own, closed after the response, and
// returns the response with its body read and closed. A fresh connection keeps the client from retrying the
// request on another one when the server drops it, as net/http does after a panic.
func GetWithoutKeepAlive(ctx context.Context, url string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, respondsWithin)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	return response, response.Body.Close()
}

// WaitForClosedConnection fails the test unless the other end closes conn within respondsWithin.
func WaitForClosedConnection(t *testing.T, conn net.Conn) {
	t.Helper()

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(respondsWithin)))
	_, err := conn.Read(make([]byte, 1))
	require.Error(t, err, "the other end sent data instead of closing the connection")
	var netErr net.Error
	require.False(t, errors.As(err, &netErr) && netErr.Timeout(), "the other end never closed the connection")
}
