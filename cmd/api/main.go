package main

import (
	"context"
	"errors"
	"flag"
	"time"

	"github.com/cpay-dev/backend-go/pkg/config"
	"github.com/cpay-dev/backend-go/pkg/db"
	"github.com/cpay-dev/backend-go/pkg/log"
	"github.com/cpay-dev/backend-go/pkg/termination"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.json", "path to config file")
	flag.Parse()

	ctx := context.Background()
	logger := log.NewZerologPretty()

	conf, err := config.LoadMaybeAtPath[Config](configPath)
	if err != nil {
		logger.Err(err).Msg("failed to parse config")
		return
	}

	dbPool, err := db.NewPgxPoolFromConn(ctx, conf.Database.ConnString(), nil, nil)
	if err != nil {
		logger.Err(err).Msg("failed to create database pool")
		return
	}

	app := &application{
		config: conf,
		logger: logger,
		dbPool: dbPool,
	}
	defer app.stop()

	appErr := make(chan error, 1)
	var startFn func() error
	var checkFn func() error

	switch conf.ServerType {
	case ServerTypeHttp:
		appErr <- errors.New("http server is not implemented")
	case ServerTypeGrpc:
		switch conf.ServerApp {
		case ServerAppMerchant:
			startFn = app.startMerchantGrpc
			checkFn = func() error {
				return app.checkMerchantGrpc(conf.ListenAddress)
			}
		}
	}

	go func() {
		appErr <- startFn()
	}()

	go func() {
		time.Sleep(time.Second * time.Duration(conf.HealthCheckDelay))
		if err := checkFn(); err != nil {
			appErr <- err
		} else {
			logger.Info().Msg("application is healthy")
		}
	}()

	select {
	case err := <-appErr:
		logger.Err(err).Msg("failed to start application")
	case <-termination.Notify():
		return
	}
}
