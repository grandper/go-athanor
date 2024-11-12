package nats

import (
	"net/http"

	"github.com/grandper/go-athanor/resource"
)

// DefaultHealthSubject is the default subject the health of the resources is served on.
const DefaultHealthSubject = "health"

// HealthHandler reports the health of a [resource.List] over NATS, on the "health" subject by default.
type HealthHandler struct {
	resources *resource.List
	subject   string
}

// NewHealthHandler returns a HealthHandler backed by the given resource List.
func NewHealthHandler(resources *resource.List) *HealthHandler {
	return &HealthHandler{resources: resources, subject: DefaultHealthSubject}
}

// WithSubject overrides the subject the health is served on.
func (h *HealthHandler) WithSubject(subject string) *HealthHandler {
	h.subject = subject
	return h
}

// healthResponse is the JSON payload published by the health handler.
type healthResponse struct {
	Status    bool              `json:"status"`
	Resources map[string]string `json:"resources"`
}

// Register implements the Handler interface.
func (h *HealthHandler) Register(server *Server) error {
	return server.HandleFunc(h.subject, "", h.handleHealth)
}

// handleHealth replies with the health of the resources, using the status codes of the HTTP health handler.
func (h *HealthHandler) handleHealth(r *Request, publish PublishFunc) error {
	summary := h.resources.StatusSummary()
	allHealthy := h.resources.IsHealthy()

	resources := make(map[string]string, len(summary))
	for name, status := range summary {
		resources[name] = status.String()
	}

	code := http.StatusOK
	if !allHealthy {
		code = http.StatusServiceUnavailable
	}

	return RespondWithJSON(publish, r.Msg, code, healthResponse{
		Status:    allHealthy,
		Resources: resources,
	})
}

// HealthHandler implements the Handler interface.
var _ Handler = (*HealthHandler)(nil)
