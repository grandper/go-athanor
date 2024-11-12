package http

import (
	"net/http"

	"github.com/gorilla/mux"
)

// VersionHandler exposes the service version information on GET /api/{apiVersion}/version.
type VersionHandler struct {
	apiVersion string
	info       VersionInfo
}

// NewVersionHandler returns a VersionHandler serving info under the given API version.
func NewVersionHandler(apiVersion string, info VersionInfo) *VersionHandler {
	return &VersionHandler{apiVersion: apiVersion, info: info}
}

// Register implements the Handler interface.
func (vh *VersionHandler) Register(router *mux.Router) error {
	router.HandleFunc("/api/"+vh.apiVersion+"/version", vh.handleVersion).Methods(http.MethodGet)
	return nil
}

// handleVersion writes the version information as JSON.
func (vh *VersionHandler) handleVersion(w http.ResponseWriter, _ *http.Request) {
	_ = RespondWithJSON(w, http.StatusOK, vh.info)
}

// VersionHandler implements the Handler interface.
var _ Handler = (*VersionHandler)(nil)
