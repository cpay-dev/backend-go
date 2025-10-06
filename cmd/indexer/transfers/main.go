package main

import (
	"context"
	"flag"
	"time"

	"github.com/cpay-dev/backend-go/internal/indexer/repo/pg"
	"github.com/cpay-dev/backend-go/internal/indexer/transfers"
	"github.com/cpay-dev/backend-go/pkg/config"
	"github.com/cpay-dev/backend-go/pkg/db"
	"github.com/cpay-dev/backend-go/pkg/log"
	"github.com/cpay-dev/backend-go/pkg/termination"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.json", "path to config file")
	flag.Parse()

	ctx, appCancel := context.WithCancel(context.Background())
	defer appCancel()
	logger := log.NewZerologPretty()

	conf, err := config.LoadMaybeAtPath[Config](configPath)
	if err != nil {
		logger.Err(err).Msg("failed to parse config")
		return
	}

	if conf.Environment > config.EnvironmentLocal {
		logger = log.NewZerologWithLevel(conf.LogLevel)
	} else {
		logger = logger.Level(conf.LogLevel)
	}

	dbPool, err := db.NewPgxPoolFromConn(ctx, conf.Database.ConnString(), nil, conf.Database.TlsConfig())
	if err != nil {
		logger.Err(err).Msg("failed to create database pool")
		return
	}

	logger.Debug().Msg("database ping...")
	pingCtx, cancel := context.WithTimeout(ctx, time.Second*5)
	if conf.Environment == config.EnvironmentLocal {
		cancel()
		pingCtx, cancel = context.WithTimeout(ctx, time.Second*30)
	}
	err = dbPool.Ping(pingCtx)
	cancel()
	if err != nil {
		logger.Err(err).Msg("failed to ping database")
		return
	}
	logger.Debug().Msg("database ready")

	app := &application{
		config: conf,
		logger: logger,
		dbPool: dbPool,
	}
	defer app.stop()

	repo := pg.NewPostgresRepo(db.NewPgxPoolWrapper(dbPool))

	var processor transfers.BlockProcessor
	switch conf.Chain {
	case ChainUnichain:
		processor = transfers.NewUnichainProcessor(repo, logger)
	default:
		logger.Error().Str("chain", string(conf.Chain)).Msg("unknown chain")
		return
	}
	app.consumer = transfers.NewConsumer(conf.Nats, logger, processor)

	appErr := make(chan error, 1)

	go func() {
		appErr <- app.run(ctx)
	}()

	select {
	case err := <-appErr:
		logger.Err(err).Msg("application failed")
	case <-termination.Notify():
		return
	}
}
