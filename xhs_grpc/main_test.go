package main

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/cloudwego/kitex/pkg/endpoint"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	health "media_agent/xhs_grpc/kitex_gen/grpc/health/v1"
	healthclient "media_agent/xhs_grpc/kitex_gen/grpc/health/v1/health"
)

func TestJWTMiddlewareAllowsGRPCHealthCheck(t *testing.T) {
	called := false
	next := endpoint.Endpoint(func(context.Context, interface{}, interface{}) error {
		called = true
		return nil
	})

	invocation := rpcinfo.NewInvocation("Health", "Check", "grpc.health.v1")
	ri := rpcinfo.NewRPCInfo(nil, nil, invocation, nil, nil)
	ctx := rpcinfo.NewCtxWithRPCInfo(context.Background(), ri)
	err := jwtMiddleware(nil)(next)(ctx, struct{}{}, nil)

	require.NoError(t, err)
	require.True(t, called)
}

func TestJWTMiddlewareStillProtectsBusinessRPC(t *testing.T) {
	called := false
	next := endpoint.Endpoint(func(context.Context, interface{}, interface{}) error {
		called = true
		return nil
	})

	err := jwtMiddleware(nil)(next)(context.Background(), struct{}{}, nil)

	require.ErrorContains(t, err, "missing gRPC metadata")
	require.False(t, called)
}

func TestGRPCHealthCheckOverNPHTTP2(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	tcpAddress, err := net.ResolveTCPAddr("tcp", address)
	require.NoError(t, err)
	kitexServer := server.NewServer(
		server.WithServiceAddr(tcpAddress),
		server.WithCompatibleMiddlewareForUnary(),
		server.WithMiddleware(jwtMiddleware(nil)),
	)
	require.NoError(t, healthclient.RegisterService(kitexServer, &healthService{}))
	go func() {
		_ = kitexServer.Run()
	}()
	t.Cleanup(func() { _ = kitexServer.Stop() })
	require.Eventually(t, func() bool {
		conn, dialErr := net.DialTimeout("tcp", address, time.Second)
		if dialErr != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 5*time.Second, 100*time.Millisecond)

	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	response := new(health.HealthCheckResponse)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = conn.Invoke(ctx, "/grpc.health.v1.Health/Check", &health.HealthCheckRequest{}, response)
	require.NoError(t, err)
	require.Equal(t, health.HealthCheckResponse_SERVING, response.Status)
}
