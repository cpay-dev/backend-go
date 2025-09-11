package migration

import "github.com/cpay-dev/backend-go/pkg/config"

type Config struct {
	Database              config.Database `json:"database"`
	ForceVersion          int             `json:"force_version" env:"FORCE_VERSION"`
	SqlSchemaDir          string          `json:"sql_schema_dir" env:"SQL_SCHEMA_DIR,notEmpty"`
	SearchPath            string          `json:"search_path" env:"SEARCH_PATH"`
	MigrationsTableQuoted string          `json:"migrations_table_quoted" env:"MIGRATIONS_TABLE_QUOTED"`
}
