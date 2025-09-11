package main

import (
	"context"
	"flag"

	"github.com/cpay-dev/backend-go/internal/api/migrate"
	"github.com/cpay-dev/backend-go/pkg/config"
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

	switch conf.Schema {
	case SchemaApp:
		migrator := migrate.NewAppMigrator(conf.Database, conf.SqlSchemaDir)
		if err := migrator.Migrate(ctx, conf.ForceVersion); err != nil {
			logger.Err(err).Msg("failed to migrate database")
			return
		}
	case SchemaBlockchain:
		migrator := migrate.NewBlockchainMigrator(conf.Database, conf.SqlSchemaDir)
		if err := migrator.Migrate(ctx, conf.ForceVersion); err != nil {
			logger.Err(err).Msg("failed to migrate database")
			return
		}
	default:
		logger.Error().Str("schema", conf.Schema).Msg("invalid schema")
		return
	}

	logger.Info().Msg("database migrated successfully")
}
