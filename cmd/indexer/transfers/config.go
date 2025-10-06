package main

import (
	"fmt"
	"strings"

	"github.com/cpay-dev/backend-go/internal/indexer/transfers"
	"github.com/cpay-dev/backend-go/pkg/config"
	"github.com/rs/zerolog"
)

type Chain string

const (
	ChainUnichain Chain = "unichain"
)

type Config struct {
	Database    config.Database          `json:"database"`
	Environment config.Environment       `json:"environment"`
	LogLevel    zerolog.Level            `json:"log_level"`
	Chain       Chain                    `json:"chain"`
	Nats        transfers.ConsumerConfig `json:"nats"`
}

func (c *Chain) UnmarshalText(text []byte) error {
	v := strings.TrimSpace(strings.ToLower(string(text)))
	switch Chain(v) {
	case ChainUnichain:
		*c = ChainUnichain
		return nil
	}
	return fmt.Errorf("invalid chain: %q", v)
}
