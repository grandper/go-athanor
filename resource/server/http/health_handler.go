package http

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/grandper/go-athanor/resource"
)

// DefaultHealthPath is the default path the health of the resources is served on.
const DefaultHealthPath = "/health"

// HealthHandler reports the health of a [resource.List] over HTTP, on GET /health by default.
type HealthHandler struct {
	resources *resource.List
	path      string
}

// NewHealthHandler returns a HealthHandler backed by the given resource List.
func NewHealthHandler(resources *resource.List) *HealthHandler {
	return &HealthHandler{resources: resources, path: DefaultHealthPath}
}

// WithPath overrides the path the health is served on.
func (h *HealthHandler) WithPath(path string) *HealthHandler {
	h.path = path
	return h
}

// Register implements the Handler interface.
func (h *HealthHandler) Register(router *mux.Router) error {
	router.Handle(h.path, h).Methods(http.MethodGet)
	return nil
}

// healthResponse is the JSON payload returned by the health endpoint.
type healthResponse struct {
	Status    bool              `json:"status"`
	Resources map[string]string `json:"resources"`
}

// ServeHTTP implements http.Handler.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	summary := h.resources.StatusSummary()
	allHealthy := h.resources.IsHealthy()

	resources := make(map[string]string, len(summary))
	for name, status := range summary {
		resources[name] = status.String()
	}

	w.Header().Set("Content-Type", "application/json")
	if !allHealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	_ = json.NewEncoder(w).Encode(healthResponse{
		Status:    allHealthy,
		Resources: resources,
	})
}

// HealthHandler implements the Handler interface.
var _ Handler = (*HealthHandler)(nil)

// HealthHandler implements the http.Handler interface.
var _ http.Handler = (*HealthHandler)(nil)
