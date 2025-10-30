package appenv

import (
	"os"

	"github.com/rs/zerolog"
)

// SplitLevelWriter writes < Warn to stdout, Warn and above to stderr.
type SplitLevelWriter struct{}

func (SplitLevelWriter) Write(p []byte) (n int, err error) { return os.Stdout.Write(p) }

func (SplitLevelWriter) WriteLevel(level zerolog.Level, p []byte) (n int, err error) {
	if level < zerolog.WarnLevel {
		return os.Stdout.Write(p)
	}
	return os.Stderr.Write(p)
}
