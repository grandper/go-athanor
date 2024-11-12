package http_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpserver "github.com/grandper/go-athanor/resource/server/http"
)

func TestVersionInfo(t *testing.T) {
	t.Run("should serialize the version, the commit, and the build date under their JSON names", func(t *testing.T) {
		info := httpserver.VersionInfo{
			Version:   "1.2.3",
			Commit:    "abc123",
			BuildDate: "2026-08-10T12:00:00Z",
		}

		data, err := json.Marshal(info)

		require.NoError(t, err)
		assert.JSONEq(t, `{"version":"1.2.3","commit":"abc123","build_date":"2026-08-10T12:00:00Z"}`, string(data))
	})

	t.Run("should read the version, the commit, and the build date from their JSON names", func(t *testing.T) {
		var info httpserver.VersionInfo
		body := `{"version":"1.2.3","commit":"abc123","build_date":"2026-08-10T12:00:00Z"}`

		err := json.Unmarshal([]byte(body), &info)

		require.NoError(t, err)
		assert.Equal(t, httpserver.VersionInfo{
			Version:   "1.2.3",
			Commit:    "abc123",
			BuildDate: "2026-08-10T12:00:00Z",
		}, info)
	})
}
