package log

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/sdcio/data-server/pkg/dslog"
	"go.uber.org/zap"
)

const (
	VDebug = 1
	VWarn  = 2
	VTrace = 8
)

var FallbackLogger logr.Logger
var DefaultLogger logr.Logger

func init() {
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(dslog.TraceLevel)
	zlog, err := config.Build()
	if err != nil {
		panic(err)
	}
	DefaultLogger = zapr.NewLogger(zlog)
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
