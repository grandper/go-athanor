package fixture_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grandper/go-athanor/resource/server/http/fixture"
)

func TestOpenHijackedConnection(t *testing.T) {
	t.Run("should return the connection a server took over with 101 Switching Protocols", func(t *testing.T) {
		address := serveHijackingRoute(t, fixture.NewHijackingHandler("/chat"))

		conn := fixture.OpenHijackedConnection(t, address, "/chat")

		assert.Equal(t, address, conn.RemoteAddr().String())
	})

	t.Run("should close the connection when the test ends", func(t *testing.T) {
		address := serveHijackingRoute(t, fixture.NewHijackingHandler("/chat"))
		var conn interface{ Close() error }
		t.Run("test using the connection", func(t *testing.T) {
			conn = fixture.OpenHijackedConnection(t, address, "/chat")
		})

		assert.Error(t, conn.Close(), "the connection must already be closed")
	})
}

func TestGetWithoutKeepAlive(t *testing.T) {
	ctx := context.Background()

	t.Run("should return the response of the server on a connection of its own", func(t *testing.T) {
		var closeRequested bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			closeRequested = r.Close
			w.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()

		response, err := fixture.GetWithoutKeepAlive(ctx, server.URL)

		require.NoError(t, err)
		assert.Equal(t, http.StatusAccepted, response.StatusCode)
		assert.True(t, closeRequested, "the request must ask for its connection to be closed")
	})

	t.Run("fails when the server drops the connection without answering", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic(http.ErrAbortHandler)
		}))
		defer server.Close()

		_, err := fixture.GetWithoutKeepAlive(ctx, server.URL)

		assert.Error(t, err)
	})

	t.Run("fails when the url is invalid", func(t *testing.T) {
		_, err := fixture.GetWithoutKeepAlive(ctx, "://missing-scheme")

		assert.Error(t, err)
	})
}

func TestWaitForClosedConnection(t *testing.T) {
	t.Run("should return once the other end closes the connection", func(t *testing.T) {
		var listenConfig net.ListenConfig
		listener, err := listenConfig.Listen(context.Background(), "tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer func() { _ = listener.Close() }()
		var dialer net.Dialer
		conn, err := dialer.DialContext(context.Background(), "tcp", listener.Addr().String())
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()
		accepted, err := listener.Accept()
		require.NoError(t, err)

		require.NoError(t, accepted.Close())

		fixture.WaitForClosedConnection(t, conn)
	})
}
