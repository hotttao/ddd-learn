package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/hotttao/ddd-learn/internal/security"
	"github.com/hotttao/ddd-learn/internal/xhsservice"
)

func main() {
	address := env("HTTP_ADDR", ":8082")
	issuer := env("INTERNAL_JWT_ISSUER", "http://auth-service:8081")
	audience := env("INTERNAL_JWT_AUDIENCE", "internal-api")
	authBaseURL := env("AUTH_SERVICE_URL", "http://auth-service:8081")
	httpClient := &http.Client{Timeout: 3 * time.Second}

	verifier := security.NewJWKSVerifier(authBaseURL+"/.well-known/jwks.json", issuer, audience, httpClient)
	authorizer := xhsservice.NewHTTPAuthorizer(authBaseURL+"/v1/authorize", httpClient)
	handler := xhsservice.NewHandler(xhsservice.New(), verifier, authorizer)

	h := server.Default(server.WithHostPorts(address))
	handler.Register(h)
	log.Printf("xhs-service listening on %s (auth-service=%s)", address, authBaseURL)
	h.Spin()
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
