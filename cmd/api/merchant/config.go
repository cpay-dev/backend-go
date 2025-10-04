package main

import (
	"time"

	"github.com/cpay-dev/backend-go/pkg/config"
	"github.com/rs/zerolog"
)

type Config struct {
	config.Server
	Environment config.Environment `json:"environment" env:"ENVIRONMENT,notEmpty" envDefault:"local"`

	LogLevel zerolog.Level   `json:"log_level" env:"LOG_LEVEL,notEmpty" envDefault:"debug"`
	Database config.Database `json:"database" env:"DATABASE,notEmpty"`

	HealthCheckDelay    int `json:"health_check_delay" env:"HEALTH_CHECK_DELAY" envDefault:"5"`
	HealthCheckInterval int `json:"health_check_interval" env:"HEALTH_CHECK_INTERVAL" envDefault:"5"`
}

func (c Config) HealthCheckIntervalDuration() time.Duration {
	return time.Second * time.Duration(c.HealthCheckInterval)
}
