package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/cpay-dev/cpay/internal/platform/db"
	"github.com/cpay-dev/cpay/internal/platform/httpserver"
	"github.com/cpay-dev/cpay/internal/platform/migrate"
	"github.com/cpay-dev/cpay/internal/platform/workers"
	"github.com/cpay-dev/cpay/internal/shared/config"
	"github.com/cpay-dev/cpay/internal/shared/logx"
)

func main() {
	cfg := config.Load("chain-observer-service")
	log := logx.New(cfg.ServiceName)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("db connect failed")
	}
	defer pool.Close()
	if err := migrate.RunUp(ctx, pool); err != nil {
		log.Fatal().Err(err).Msg("migrations failed")
	}

	httpserver.StartHealthServer(ctx, cfg.HTTPAddr, cfg.ServiceName, log)
	worker := workers.ChainObserver{DB: pool, Log: log, Interval: cfg.WorkerInterval, Source: cfg.ServiceName}
	worker.Run(ctx)
}
