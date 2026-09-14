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
	kitexmetadata "github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/metadata"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	serverjwt "media_agent/hertz_infra/serverhertz/jwt"

	healthservice "media_agent/xhs_grpc/kitex_gen/grpc/health/v1/health"
	"media_agent/xhs_grpc/kitex_gen/xhs_service/crawl/crawlservice"
	"media_agent/xhs_grpc/kitex_gen/xhs_service/organization/organizationservice"
)

func main() {
	ctx := context.Background()
	validator, err := newValidator(ctx)
	if err != nil {
		log.Fatalf("initialize internal JWT validator: %v", err)
	}

	keto, err := newKetoMembershipReader(getenv("KETO_READ_URL", "http://keto-read:80"))
	if err != nil {
		log.Fatalf("initialize Keto client: %v", err)
	}
	service := newCrawlService(keto)
	organizations := &organizationService{memberships: keto}
	options := []server.Option{
		server.WithServiceAddr(&net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 8090}),
		// Keep Kitex's default protocol detector. It recognizes the HTTP/2
		// preface and selects nphttp2 for gRPC requests.
		// Protobuf unary RPCs use Kitex's StreamingUnary transport path. Convert
		// them to ordinary args/results before invoking endpoint middleware.
		server.WithCompatibleMiddlewareForUnary(),
		server.WithMiddleware(jwtMiddleware(validator)),
	}
	kitexServer := server.NewServer(options...)
	if err := crawlservice.RegisterService(kitexServer, service); err != nil {
		log.Fatalf("register crawl service: %v", err)
	}
	if err := organizationservice.RegisterService(kitexServer, organizations); err != nil {
		log.Fatalf("register organization service: %v", err)
	}
	if err := healthservice.RegisterService(kitexServer, &healthService{}); err != nil {
		log.Fatalf("register health service: %v", err)
	}
	if err := kitexServer.Run(); err != nil {
		log.Fatalf("run xhs grpc server: %v", err)
	}
}

func newValidator(ctx context.Context) (*serverjwt.Validator, error) {
	config := serverjwt.Config{
		Issuer:            getenv("INTERNAL_JWT_ISSUER", "oathkeeper"),
		Audiences:         []string{getenv("INTERNAL_JWT_AUDIENCE", "internal-api")},
		JWKSURL:           getenv("INTERNAL_JWKS_URL", "http://127.0.0.1:4456/.well-known/jwks.json"),
		AllowedAlgorithms: []string{"RS256"},
		RefreshInterval:   5 * time.Minute,
		HTTPTimeout:       2 * time.Second,
		ClockSkew:         30 * time.Second,
	}

	var err error
	for attempt := 1; attempt <= 10; attempt++ {
		var validator *serverjwt.Validator
		validator, err = serverjwt.NewValidator(ctx, config)
		if err == nil {
			return validator, nil
		}
		if attempt == 10 {
			break
		}
		log.Printf("internal JWT validator is not ready (attempt %d/10): %v; retrying", attempt, err)
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("initialize internal JWT validator after 10 attempts: %w", err)
}

func jwtMiddleware(validator *serverjwt.Validator) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request, response interface{}) error {
			// Kubernetes gRPC Probe calls /grpc.health.v1.Health/Check without
			// application credentials. Match Kitex's parsed RPC identity instead of
			// the request Go type: nphttp2 wraps unary gRPC arguments as streaming.Args.
			if isGRPCHealthCheck(ctx) {
				return next(ctx, request, response)
			}

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

func isGRPCHealthCheck(ctx context.Context) bool {
	ri := rpcinfo.GetRPCInfo(ctx)
	if ri == nil || ri.Invocation() == nil {
		return false
	}
	invocation := ri.Invocation()
	return invocation.PackageName() == "grpc.health.v1" &&
		invocation.ServiceName() == "Health" &&
		invocation.MethodName() == "Check"
}

func getenv(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
