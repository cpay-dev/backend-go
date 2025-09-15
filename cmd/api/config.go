package main

import (
	"github.com/cpay-dev/backend-go/pkg/config"
	"github.com/rs/zerolog"
)

type (
	ServerType string
	ServerApp  string
)

const (
	ServerTypeHttp ServerType = "http"
	ServerTypeGrpc ServerType = "grpc"

	ServerAppMerchant ServerApp = "merchant"
)

type Config struct {
	config.Server
	Environment config.Environment `json:"environment" env:"ENVIRONMENT,notEmpty" envDefault:"local"`

	LogLevel zerolog.Level   `json:"log_level" env:"LOG_LEVEL,notEmpty" envDefault:"debug"`
	Database config.Database `json:"database" env:"DATABASE,notEmpty"`

	ServerType ServerType `json:"server_type" env:"SERVER_TYPE,notEmpty"`
	ServerApp  ServerApp  `json:"server_app" env:"SERVER_APP,notEmpty"`

	HealthCheckDelay    int `json:"health_check_delay" env:"HEALTH_CHECK_DELAY" envDefault:"5"`
	HealthCheckInterval int `json:"health_check_interval" env:"HEALTH_CHECK_INTERVAL" envDefault:"5"`
}
