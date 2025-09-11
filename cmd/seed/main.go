package main

import (
	"context"
	"flag"

	"github.com/cpay-dev/backend-go/internal/api/repo/pg/app"
	"github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
	"github.com/cpay-dev/backend-go/internal/api/repo/pg/seed"
	"github.com/cpay-dev/backend-go/pkg/config"
	"github.com/cpay-dev/backend-go/pkg/db"
	"github.com/cpay-dev/backend-go/pkg/log"
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
	defer dbPool.Close()

	switch conf.Schema {
	case SchemaApp:
		_, err = dbPool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS app;")
		if err != nil {
			logger.Err(err).Msg("failed to create app schema")
			return
		}
		appRepo := app.NewPostgresRepo(db.NewPgxPoolWrapper(dbPool))
		seeder := seed.NewMerchantSeeder(appRepo)
		if err := seeder.Seed(ctx); err != nil {
			logger.Err(err).Msg("failed to seed database")
			return
		}
	case SchemaBlockchain:
		_, err = dbPool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS blockchain;")
		if err != nil {
			logger.Err(err).Msg("failed to create blockchain schema")
			return
		}
		blockchainRepo := blockchain.NewPostgresRepo(db.NewPgxPoolWrapper(dbPool))
		seeder := seed.NewBlockchainSeeder(blockchainRepo)
		if err := seeder.Seed(ctx); err != nil {
			logger.Err(err).Msg("failed to seed database")
			return
		}
	default:
		logger.Error().Str("schema", conf.Schema).Msg("invalid schema")
	}
}
