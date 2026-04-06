package migrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultMigrationsDir = "migrations"

// RunUp applies raw SQL migrations using golang-migrate.
func RunUp(ctx context.Context, pool *pgxpool.Pool) error {
	_ = ctx

	migrationsDir, err := resolveMigrationsDir()
	if err != nil {
		return err
	}
	sourceURL := "file://" + filepath.ToSlash(migrationsDir)
	databaseURL := strings.TrimSpace(pool.Config().ConnConfig.ConnString())
	if databaseURL == "" {
		return fmt.Errorf("database connection string is empty")
	}

	m, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		return fmt.Errorf("init golang-migrate: %w", err)
	}
	defer func() {
		_, _ = m.Close()
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run migrate up: %w", err)
	}
	return nil
}

func resolveMigrationsDir() (string, error) {
	candidates := []string{}
	if v := strings.TrimSpace(os.Getenv("MIGRATIONS_PATH")); v != "" {
		candidates = append(candidates, v)
	}
	candidates = append(candidates, "./"+defaultMigrationsDir, "/"+defaultMigrationsDir)

	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		fi, err := os.Stat(abs)
		if err == nil && fi.IsDir() {
			return abs, nil
		}
	}
	return "", fmt.Errorf("migrations directory not found (checked %s); set MIGRATIONS_PATH", strings.Join(candidates, ", "))
}
