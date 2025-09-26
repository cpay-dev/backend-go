package main

import (
	"context"
	"flag"
	"fmt"
	"time"

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

	if err := run(ctx, conf); err != nil {
		logger.Err(err).Msg("failed to migrate database")
		return
	}

	logger.Info().Msg("database migrated successfully")
}

func run(ctx context.Context, conf Config) error {
	migrator := migrate.NewMigrator(migrate.Config{
		Database:     conf.Database,
		ForceVersion: conf.ForceVersion,
		SqlSchemaDir: conf.SqlSchemaDir,
		Schema:       conf.Schema,
	})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
