package main

import (
	appenv "github.com/cpay-dev/backend-go/pkg/appenv"
	"github.com/cpay-dev/backend-go/pkg/config"
	"github.com/rs/zerolog"
)

// ServiceConfig defines the Authn service configuration using snake_case JSON tags.
type ServiceConfig struct {
	ListenAddress string             `json:"listen_address"`
	Environment   appenv.Environment `json:"environment"`
	LogLevel      zerolog.Level      `json:"log_level"`
	Providers     ProvidersSection   `json:"providers"`
	Valkey        ValkeyConfig       `json:"valkey"`
}

type ProvidersSection struct {
	Google GoogleProviderConfig `json:"google"`
}

type GoogleProviderConfig struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURI  string   `json:"redirect_uri"`
	Scopes       []string `json:"scopes"`
}

type ValkeyConfig struct {
	Address  string `json:"address"`
	Password string `json:"password"`
}

// LoadServiceConfig uses the shared config loader to read JSON into ServiceConfig.
func LoadServiceConfig() (ServiceConfig, error) {
	return config.LoadFromFlagOrDefault[ServiceConfig]()
}
