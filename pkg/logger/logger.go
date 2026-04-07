package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

// New returns a configured zerolog.Logger.
//
// In development (env == "development") it uses a coloured ConsoleWriter with
// shortened caller paths and a human-readable time format.
// In all other environments it emits structured JSON  ready for Datadog / Loki.
//
// Every log line carries:
//
//	service = APP_NAME    (e.g. "evm-sim-api")
//	build   = APP_VERSION (e.g. "1.0.0")
//	env     = APP_ENV     (e.g. "development")
func New(level, appName, version, env string) zerolog.Logger {
	// ── Level ────────────────────────────────────────────────────────────────
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"WARN: invalid LOG_LEVEL %q (defaulting to info). Must be debug|info|warn|error\n",
			level,
		)
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	// ── Caller format: base file name + line number only ─────────────────────
	zerolog.CallerMarshalFunc = func(_ uintptr, file string, line int) string {
		return fmt.Sprintf("%s:%d", filepath.Base(file), line)
	}

	zerolog.TimeFieldFormat = time.RFC3339Nano

	// ── Output writer ─────────────────────────────────────────────────────────
	var output io.Writer
	if env == "development" {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
			FormatLevel: func(i interface{}) string {
				return fmt.Sprintf("| %-5s|", i)
			},
			FormatMessage: func(i interface{}) string {
				return fmt.Sprintf("%s", i)
			},
			FormatCaller: func(i interface{}) string {
				return fmt.Sprintf("%s >", i)
			},
		}
	} else {
		output = os.Stdout // raw JSON for log aggregators
	}

	// ── Build logger ──────────────────────────────────────────────────────────
	return zerolog.New(output).
		Level(lvl).
		With().
		Timestamp().
		Caller().
		Str("service", appName).
		Str("build", version).
		Str("env", env).
		Logger()
}
