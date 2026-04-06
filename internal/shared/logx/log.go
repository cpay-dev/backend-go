package logx

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

func New(service string) zerolog.Logger {
	lvl := parseLevel(os.Getenv("LOG_LEVEL"))
	zerolog.SetGlobalLevel(lvl)
	zerolog.TimeFieldFormat = time.RFC3339Nano
	return zerolog.New(os.Stdout).With().Timestamp().Str("service", service).Logger()
}

func parseLevel(v string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info", "":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}
