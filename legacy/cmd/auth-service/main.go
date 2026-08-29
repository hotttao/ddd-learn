package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/hotttao/ddd-learn/internal/authservice"
	"github.com/hotttao/ddd-learn/internal/security"
)

func main() {
	address := env("HTTP_ADDR", ":8081")
	issuer := env("INTERNAL_JWT_ISSUER", "http://auth-service:8081")
	audience := env("INTERNAL_JWT_AUDIENCE", "internal-api")
	tokenTTL := durationEnv("INTERNAL_JWT_TTL", 5*time.Minute)
	sessionTTL := durationEnv("SESSION_TTL", 24*time.Hour)

	signer, err := security.NewEphemeralSigner(issuer, audience, tokenTTL)
	if err != nil {
		log.Fatalf("initialize signer: %v", err)
	}
	service := authservice.New(signer, sessionTTL)
	handler := authservice.NewHandler(service, boolEnv("COOKIE_SECURE", false))

	h := server.Default(server.WithHostPorts(address))
	handler.Register(h)
	log.Printf("auth-service listening on %s (issuer=%s audience=%s)", address, issuer, audience)
	h.Spin()
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		log.Fatalf("invalid %s: %v", key, err)
	}
	return parsed
}

func boolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		log.Fatalf("invalid %s: %v", key, err)
	}
	return parsed
}
