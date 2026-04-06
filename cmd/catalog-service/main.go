package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/cpay-dev/cpay/internal/platform/httpserver"
	"github.com/cpay-dev/cpay/internal/shared/config"
	"github.com/cpay-dev/cpay/internal/shared/logx"
)

func main() {
	cfg := config.Load("catalog-service")
	log := logx.New(cfg.ServiceName)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	httpserver.StartHealthServer(ctx, cfg.HTTPAddr, cfg.ServiceName, log)
	<-ctx.Done()
}
