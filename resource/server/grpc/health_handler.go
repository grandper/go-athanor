package grpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"github.com/grandper/go-athanor/resource"
)

// HealthHandler reports the health of a [resource.List] over the standard gRPC health checking protocol.
// The empty service name stands for the whole List, and every resource is a service named after it.
type HealthHandler struct {
	grpc_health_v1.UnimplementedHealthServer

	resources *resource.List
}

// NewHealthHandler returns a HealthHandler backed by the given resource List.
func NewHealthHandler(resources *resource.List) *HealthHandler {
	return &HealthHandler{resources: resources}
}

// Register implements the Handler interface.
func (hh *HealthHandler) Register(grpcServer *grpc.Server) error {
	grpc_health_v1.RegisterHealthServer(grpcServer, hh)
	return nil
}

// Check implements grpc_health_v1.HealthServer. It fails with codes.NotFound for an unknown service.
func (hh *HealthHandler) Check(_ context.Context, request *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	servingStatus := hh.servingStatus(request.GetService())
	if servingStatus == grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN {
		return nil, status.Error(codes.NotFound, "unknown service")
	}
	return &grpc_health_v1.HealthCheckResponse{Status: servingStatus}, nil
}

// Watch implements grpc_health_v1.HealthServer. It streams the serving status of the service, then every
// change of it, until the watcher leaves. The resources registered after the stream opened are not watched.
func (hh *HealthHandler) Watch(request *grpc_health_v1.HealthCheckRequest,
	stream grpc_health_v1.Health_WatchServer,
) error {
	// Observers run synchronously inside the status change, so they only signal the stream.
	changed := make(chan struct{}, 1)
	signal := resource.StatusObserverFunc(func(resource.Status) {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	// Observing before the first read guarantees that no change is missed.
	defer hh.observeResources(signal)()

	// The zero value is the UNKNOWN status, which is never reported, so the first status is always sent.
	var last grpc_health_v1.HealthCheckResponse_ServingStatus
	for {
		current := hh.servingStatus(request.GetService())
		if current != last {
			if err := stream.Send(&grpc_health_v1.HealthCheckResponse{Status: current}); err != nil {
				return status.Error(codes.Canceled, "stream has ended")
			}
			last = current
		}

		select {
		case <-stream.Context().Done():
			return status.Error(codes.Canceled, "stream has ended")
		case <-changed:
		}
	}
}

// observeResources registers observer on every resource of the List and returns a function that removes it.
func (hh *HealthHandler) observeResources(observer resource.StatusObserver) func() {
	var removals []func()
	for name := range hh.resources.StatusSummary() {
		if observable, ok := hh.resources.StatusOf(name); ok {
			removals = append(removals, observable.Observe(observer))
		}
	}
	return func() {
		for _, remove := range removals {
			remove()
		}
	}
}

// servingStatus maps the health of the List, or of the resource named service, to a serving status.
func (hh *HealthHandler) servingStatus(service string) grpc_health_v1.HealthCheckResponse_ServingStatus {
	operational := hh.resources.IsHealthy()
	if service != "" {
		observable, ok := hh.resources.StatusOf(service)
		if !ok {
			return grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN
		}
		operational = observable.Get().IsOperational()
	}

	if operational {
		return grpc_health_v1.HealthCheckResponse_SERVING
	}
	return grpc_health_v1.HealthCheckResponse_NOT_SERVING
}

// HealthHandler implements the Handler interface.
var _ Handler = (*HealthHandler)(nil)

// HealthHandler implements the grpc_health_v1.HealthServer interface.
var _ grpc_health_v1.HealthServer = (*HealthHandler)(nil)
