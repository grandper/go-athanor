package fixture

import (
	"context"
	"testing"

	athanorfixture "github.com/grandper/go-athanor/fixture"
	httpserver "github.com/grandper/go-athanor/resource/server/http"
)

// InitOnFreePort sets a free TCP port on config and calls initialize, typically the Init of the server built
// from config. The HTTP server binds its port itself, so another process can take the port first: initialize
// is then called again with another port, as athanorfixture.OnFreePort does.
func InitOnFreePort(
	ctx context.Context, t *testing.T, config *httpserver.Config, initialize func(context.Context) error,
) error {
	t.Helper()

	return athanorfixture.OnFreePort(t, func(port int) error {
		config.BindPort = port
		return initialize(ctx)
	})
}
