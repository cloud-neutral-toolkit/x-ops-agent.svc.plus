package main

import (
	"context"
	"log"

	"github.com/yourname/ops-agent-poc/internal/config"
	"github.com/yourname/ops-agent-poc/internal/server"
	"github.com/yourname/ops-agent-poc/pkg/telemetry"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	shutdown, err := telemetry.Init(ctx, "aiops", cfg.OtlpEndpoint)
	if err != nil {
		log.Fatalf("failed to init telemetry: %v", err)
	}
	defer func() { _ = shutdown(ctx) }()

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("server init: %v", err)
	}
	if err := srv.Run(ctx); err != nil {
		log.Fatalf("server run: %v", err)
	}
}
