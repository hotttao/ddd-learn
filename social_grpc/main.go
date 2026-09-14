package main

import (
	"log"
	"net"

	"github.com/cloudwego/kitex/server"
	// Register Kitex's gRPC gzip compressor for native Kubernetes gRPC probes.
	_ "github.com/cloudwego/kitex/pkg/remote/codec/protobuf/encoding/gzip"

	socialservice "media_agent/social_grpc/kitex_gen/social_service/socialservice"
)

func main() {
	service := newSocialService(mockXHSProvider{})
	kitexServer := server.NewServer(
		server.WithServiceAddr(&net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 8091}),
		// Keep Kitex's default protocol detector so the server accepts gRPC
		// requests through Kitex's nphttp2 transport.
		server.WithCompatibleMiddlewareForUnary(),
	)
	if err := socialservice.RegisterService(kitexServer, service); err != nil {
		log.Fatalf("register social service: %v", err)
	}
	if err := kitexServer.Run(); err != nil {
		log.Fatalf("run social grpc server: %v", err)
	}
}
