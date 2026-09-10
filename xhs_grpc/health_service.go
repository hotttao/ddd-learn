package main

import (
	"context"

	health "media_agent/xhs_grpc/kitex_gen/grpc/health/v1"
)

// healthService implements the standard gRPC Health Check RPC used by
// Kubernetes gRPC probes. The service is ready as soon as the Kitex server is
// able to dispatch requests to this handler.
type healthService struct{}

func (healthService) Check(context.Context, *health.HealthCheckRequest) (*health.HealthCheckResponse, error) {
	return &health.HealthCheckResponse{Status: health.HealthCheckResponse_SERVING}, nil
}
