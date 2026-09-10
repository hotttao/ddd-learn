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
	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2"
	kitexmetadata "github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/metadata"
	"github.com/cloudwego/kitex/server"
	serverjwt "media_agent/hertz_infra/serverhertz/jwt"

	healthservice "media_agent/xhs_grpc/kitex_gen/grpc/health/v1/health"
	"media_agent/xhs_grpc/kitex_gen/xhs_service/crawl/crawlservice"
)

func main() {
	ctx := context.Background()
	validator, err := newValidator(ctx)
	if err != nil {
		log.Fatalf("initialize internal JWT validator: %v", err)
	}

	service := newCrawlService()
	options := []server.Option{
		server.WithServiceAddr(&net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 8090}),
		server.WithTransHandlerFactory(nphttp2.NewSvrTransHandlerFactory()),
		server.WithMiddleware(jwtMiddleware(validator)),
	}
	kitexServer := server.NewServer(options...)
	if err := crawlservice.RegisterService(kitexServer, service); err != nil {
		log.Fatalf("register crawl service: %v", err)
	}
	if err := healthservice.RegisterService(kitexServer, healthService{}); err != nil {
		log.Fatalf("register health service: %v", err)
	}
	if err := kitexServer.Run(); err != nil {
		log.Fatalf("run xhs grpc server: %v", err)
	}
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
			md, ok := kitexmetadata.FromIncomingContext(ctx)
			if !ok {
				return fmt.Errorf("missing gRPC metadata")
			}
			values := md.Get("authorization")
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
