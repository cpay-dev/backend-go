package migrate

import (
	"context"
	"fmt"

	"github.com/cpay-dev/backend-go/pkg/config"
	"github.com/cpay-dev/backend-go/pkg/db"
	"github.com/cpay-dev/backend-go/pkg/db/migration"
)

type AppMigrator struct {
	config       config.Database
	sqlSchemaDir string
}

func NewAppMigrator(config config.Database, sqlSchemaDir string) *AppMigrator {
	return &AppMigrator{config: config, sqlSchemaDir: sqlSchemaDir}
}

func (m *AppMigrator) Migrate(ctx context.Context, version int) error {
	dbPool, err := db.NewPgxPoolFromConn(ctx, m.config.ConnString(), nil, nil)
	if err != nil {
		return fmt.Errorf("create database pool: %w", err)
	}
	defer dbPool.Close()

	_, err = dbPool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS app;")
	if err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	migrator := migration.NewPostgresMigrator()
	return migrator.Migrate(ctx, migration.Config{
		Database:     m.config,
		ForceVersion: version,
		SqlSchemaDir: m.sqlSchemaDir,
		SearchPath:   "app",
	})
}
