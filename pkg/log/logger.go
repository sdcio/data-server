package log

import (
	"context"
	"log/slog"
	"os"

	"github.com/go-logr/logr"
)

const (
	VTrace = -8
	VDebug = slog.LevelDebug
	VWarn  = slog.LevelWarn
	VError = slog.LevelError
)

var FallbackLogger logr.Logger
var DefaultLogger logr.Logger

func init() {
	DefaultLogger = logr.FromSlogHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(VTrace)}))
	FallbackLogger = DefaultLogger.WithName("fallback-logger")
}

func SetDefaultLogger(logger logr.Logger) {
	DefaultLogger = logger
}

func FromContext(ctx context.Context) logr.Logger {
	log := FallbackLogger
	if cLog, err := logr.FromContext(ctx); err == nil {
		log = cLog
	}
	return log
}

func IntoContext(ctx context.Context, logger logr.Logger) context.Context {
	return logr.NewContext(ctx, logger)
}
