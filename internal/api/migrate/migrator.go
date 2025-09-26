package migrate

import (
	"context"
	"fmt"

	"github.com/cpay-dev/backend-go/pkg/config"
	"github.com/cpay-dev/backend-go/pkg/db"
	"github.com/cpay-dev/backend-go/pkg/db/migration"
	"github.com/jackc/pgx/v5"
)

type Migrator struct {
	config Config
}

type Config struct {
	Database     config.Database
	ForceVersion int
	SqlSchemaDir string
	Schema       string
}

func NewMigrator(config Config) *Migrator {
	return &Migrator{config: config}
}

func (m *Migrator) Migrate(ctx context.Context) error {
	dbPool, err := db.NewPgxPoolFromConn(ctx, m.config.Database.ConnString(), nil, nil)
	if err != nil {
		return fmt.Errorf("create database pool: %w", err)
	}
	defer dbPool.Close()

	_, err = dbPool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+pgx.Identifier{m.config.Schema}.Sanitize()+";")
	if err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	migrator := migration.NewPostgresMigrator()
	return migrator.Migrate(ctx, migration.Config{
		Database:              m.config.Database,
		ForceVersion:          m.config.ForceVersion,
		SqlSchemaDir:          m.config.SqlSchemaDir,
		SearchPath:            m.config.Schema,
		MigrationsTableQuoted: pgx.Identifier{m.config.Schema, "schema_migrations"}.Sanitize(),
	})
}
