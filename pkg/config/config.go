package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/caarlos0/env/v11"
)

func Load[T any]() (T, error) {
	return LoadMaybeAtPath[T]("config.json")
}

func LoadMaybeAtPath[T any](path string) (T, error) {
	conf, err := LoadAtPath[T](path)
	if errors.Is(err, os.ErrNotExist) {
		return LoadFromEnv[T]()
	} else if err != nil {
		return conf, fmt.Errorf("load config: %w", err)
	}
	return conf, nil
}

func LoadAtPath[T any](path string) (T, error) {
	var conf T
	bb, err := os.ReadFile(path)
	if err != nil {
		return conf, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(bb, &conf); err != nil {
		return conf, fmt.Errorf("unmarshal config: %w", err)
	}
	return conf, nil
}

func LoadFromEnv[T any]() (T, error) {
	conf, err := env.ParseAs[T]()
	if err != nil {
		return conf, fmt.Errorf("parse config from env: %w", err)
	}
	return conf, nil
}
