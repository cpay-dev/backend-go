package main

import (
	"github.com/cpay-dev/backend-go/pkg/config"
)

type Config struct {
	Database     config.Database `json:"database"`
	ForceVersion int             `json:"force_version"`
	SqlSchemaDir string          `json:"sql_schema_dir"`
	Schema       string          `json:"schema"`
}
