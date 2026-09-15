package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/kitex/pkg/endpoint"
	"github.com/cloudwego/kitex/server"
	// Register Kitex's gRPC gzip compressor for native Kubernetes gRPC probes.
	_ "github.com/cloudwego/kitex/pkg/remote/codec/protobuf/encoding/gzip"
	kitexmetadata "github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/metadata"
	serverjwt "media_agent/hertz_infra/serverhertz/jwt"

	socialservice "media_agent/social_grpc/kitex_gen/social_service/socialservice"
)

func main() {
	validator, err := newValidatorWithRetry(context.Background())
	if err != nil {
		log.Fatalf("initialize internal JWT validator: %v", err)
	}
	provider, err := newXHSProvider(getenv("XHS_GRPC_ADDR", "xhs-grpc-service:80"))
	if err != nil {
		log.Fatalf("initialize XHS provider: %v", err)
	}
	service := newSocialService(provider)
	kitexServer := server.NewServer(
		server.WithServiceAddr(&net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 8091}),
		// Keep Kitex's default protocol detector so the server accepts gRPC
		// requests through Kitex's nphttp2 transport.
		server.WithCompatibleMiddlewareForUnary(),
		server.WithMiddleware(jwtMiddleware(validator)),
	)
	if err := socialservice.RegisterService(kitexServer, service); err != nil {
		log.Fatalf("register social service: %v", err)
	}
	if err := kitexServer.Run(); err != nil {
		log.Fatalf("run social grpc server: %v", err)
	}
}

func newValidatorWithRetry(ctx context.Context) (*serverjwt.Validator, error) {
	var err error
	for attempt := 1; attempt <= 5; attempt++ {
		var validator *serverjwt.Validator
		validator, err = newValidator(ctx)
		if err == nil {
			return validator, nil
		}
		if attempt < 5 {
			log.Printf("initialize internal JWT validator (attempt %d/5): %v; retrying", attempt, err)
			time.Sleep(2 * time.Second)
		}
	}
	return nil, err
}

func newValidator(ctx context.Context) (*serverjwt.Validator, error) {
	return serverjwt.NewValidator(ctx, serverjwt.Config{
		Issuer:            getenv("INTERNAL_JWT_ISSUER", "oathkeeper"),
		Audiences:         []string{getenv("INTERNAL_JWT_AUDIENCE", "internal-api")},
		JWKSURL:           getenv("INTERNAL_JWKS_URL", "http://127.0.0.1:4456/.well-known/jwks.json"),
		AllowedAlgorithms: []string{"RS256"},
		RefreshInterval:   5 * time.Minute,
		HTTPTimeout:       2 * time.Second,
		ClockSkew:         30 * time.Second,
	})
}

func jwtMiddleware(validator *serverjwt.Validator) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request, response interface{}) error {
			metadata, ok := kitexmetadata.FromIncomingContext(ctx)
			if !ok {
				return fmt.Errorf("missing gRPC metadata")
			}
			values := metadata.Get("authorization")
			if len(values) == 0 {
				return fmt.Errorf("missing authorization metadata")
			}
			raw, err := serverjwt.BearerToken(values[0])
			if err != nil {
				return err
			}
			principal, err := validator.Validate(ctx, raw)
			if err != nil {
				return err
			}
			return next(serverjwt.WithPrincipal(ctx, principal), request, response)
		}
	}
}

func getenv(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
