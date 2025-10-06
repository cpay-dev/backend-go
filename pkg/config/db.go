package config

import (
	"crypto/tls"
	"fmt"
)

type Database struct {
	Host     string `json:"host" env:"DB_HOST,notEmpty,unset"`
	Domain   string `json:"domain" env:"DB_DOMAIN,unset"`
	Username string `json:"username" env:"DB_USERNAME,notEmpty,unset"`
	Password string `json:"password" env:"DB_PASSWORD,notEmpty,unset"`
	Database string `json:"database" env:"DB_DATABASE,notEmpty,unset"`
	SSLMode  string `json:"ssl_mode" env:"DB_SSL_MODE" envDefault:"require"`
}

func (d Database) ConnString() string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s", d.Username, d.Password, d.Host, d.Database, d.SSLMode)
}

func (d Database) TlsConfig() *tls.Config {
	cfg := &tls.Config{}
	switch d.SSLMode {
	case "", "none", "disable":
		return nil
	}
	if d.Domain != "" {
		cfg.ServerName = d.Domain
	}
	return cfg
}
