package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	health "media_agent/xhs_grpc/kitex_gen/grpc/health/v1"
)

func main() {
	address := "127.0.0.1:18090"
	if len(os.Args) > 1 {
		address = os.Args[1]
	}

	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("create gRPC connection: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response := new(health.HealthCheckResponse)
	err = conn.Invoke(ctx, "/grpc.health.v1.Health/Check", &health.HealthCheckRequest{}, response)
	if err != nil {
		log.Fatalf("check health: %v", err)
	}
	fmt.Printf("health status=%s\n", response.Status.String())
}
