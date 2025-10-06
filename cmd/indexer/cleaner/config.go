package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/cpay-dev/backend-go/pkg/config"
	"github.com/rs/zerolog"
)

type Interval time.Duration

type Config struct {
	Database    config.Database    `json:"database"`
	Environment config.Environment `json:"environment"`
	LogLevel    zerolog.Level      `json:"log_level"`
	Interval    Interval           `json:"interval"`
}

func (c *Interval) UnmarshalText(text []byte) error {
	v := strings.TrimSpace(strings.ToLower(string(text)))
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("invalid interval: %q: %w", v, err)
	}
	*c = Interval(d)
	return nil
}

func (c Interval) Duration() time.Duration {
	return time.Duration(c)
}
