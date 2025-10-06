package main

import (
	"time"

	"github.com/cpay-dev/backend-go/pkg/config"
	"github.com/rs/zerolog"
)

type Config struct {
	config.Server
	Environment config.Environment `json:"environment"`

	LogLevel              zerolog.Level   `json:"log_level"`
	Database              config.Database `json:"database" `
	WalletServiceEndpoint string          `json:"wallet_service_endpoint"`

	HealthCheckDelay    int `json:"health_check_delay"`
	HealthCheckInterval int `json:"health_check_interval"`
}

func (c Config) HealthCheckIntervalDuration() time.Duration {
	return time.Second * time.Duration(c.HealthCheckInterval)
}
