package log

import (
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

var DefaultRoots = []string{
	"cmd/",
	"internal/",
	"pkg/",
	"testing/",
}

func NewZerologWithLevel(lvl zerolog.Level) zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.TimestampFieldName = "timestamp"
	zerolog.LevelFieldName = "severity"
	zerolog.MessageFieldName = "message"
	zerolog.DurationFieldInteger = true
	return zerolog.New(
		ZerologLevelToWriter{
			m: map[zerolog.Level]io.Writer{
				zerolog.WarnLevel:  os.Stderr,
				zerolog.ErrorLevel: os.Stderr,
				zerolog.FatalLevel: os.Stderr,
				zerolog.PanicLevel: os.Stderr,
			},
		},
	).With().Timestamp().Caller().Stack().Logger().Level(lvl)
}

func NewZerologPretty() zerolog.Logger {
	wr := zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.TimeFormat = time.RFC3339
	})

	logger := zerolog.New(wr).
		With().
		Timestamp().
		Caller().
		Logger()

	return logger
}

func ZerologSetupWithRoots(roots ...string) {
	if len(roots) == 0 {
		roots = DefaultRoots
	}
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		for _, root := range roots {
			if i := strings.Index(file, root); i > 0 {
				file = file[i:]
				break
			}
		}
		return file + ":" + strconv.Itoa(line)
	}
}

type ZerologLevelToWriter struct {
	io.Writer
	m map[zerolog.Level]io.Writer
}

func (w ZerologLevelToWriter) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	if dst, ok := w.m[level]; ok {
		return dst.Write(p)
	}
	return os.Stdout.Write(p)
}
