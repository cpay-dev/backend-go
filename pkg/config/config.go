package config

import (
	"flag"
	"fmt"
	"os"

	"github.com/goccy/go-json"
)

// LoadFromFlagOrDefault is a generic JSON loader that reads -config if provided,
// otherwise falls back to ./config.json and unmarshals into the target type T.
func LoadFromFlagOrDefault[T any]() (T, error) {
	var path string
	if f := flag.Lookup("config"); f != nil {
		path = f.Value.String()
	} else {
		path = "./config.json"
	}
	return Load[T](path)
}

// Load reads a JSON file into the provided type.
func Load[T any](path string) (T, error) {
	var cfg T
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("failed to read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return cfg, nil
}
