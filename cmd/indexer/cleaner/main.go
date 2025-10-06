package main

import (
	"context"
	"flag"
	"time"

	"github.com/cpay-dev/backend-go/internal/indexer/cleaner"
	"github.com/cpay-dev/backend-go/internal/indexer/repo/pg"
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
	}

	if conf.Interval == 0 {
		logger.Error().Msg("clean interval is not set")
		return
	}

	dbPool, err := db.NewPgxPoolFromConn(ctx, conf.Database.ConnString(), nil, nil)
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

	repo := pg.NewPostgresRepo(db.NewPgxPoolWrapper(dbPool))
	app := &application{
		config:  conf,
		logger:  logger,
		dbPool:  dbPool,
		cleaner: cleaner.NewService(repo),
	}
	defer app.stop()

	go func() {
		<-termination.Notify()
		appCancel()
	}()

	if err := app.run(ctx, conf.Interval.Duration()); err != nil {
		logger.Err(err).Msg("application error")
	}
}
