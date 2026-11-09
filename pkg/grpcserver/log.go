package grpcserver

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/sdcio/data-server/pkg/utils"
	logf "github.com/sdcio/logger"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// begin names the handler logger, stores it in the returned context and logs
// the redacted request at debug level.
func begin(ctx context.Context, name string, req proto.Message, kv ...any) (context.Context, logr.Logger) {
	return beginWith(ctx, logf.FromContext(ctx).WithName(name).WithValues(kv...), req)
}

func beginWith(ctx context.Context, log logr.Logger, req proto.Message) (context.Context, logr.Logger) {
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))
	return logf.IntoContext(ctx, log), log
}

// requestLogger returns the base logger tagged with a fresh request id.
func requestLogger(base context.Context) logr.Logger {
	return logf.FromContext(base).WithValues("grpc-request-uuid", uuid.New().String())
}

func loggingUnaryInterceptor(base context.Context) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(logf.IntoContext(ctx, requestLogger(base)), req)
	}
}

func loggingStreamInterceptor(base context.Context) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		wss := grpc_middleware.WrapServerStream(ss)
		wss.WrappedContext = logf.IntoContext(wss.WrappedContext, requestLogger(base))
		return handler(srv, wss)
	}
}
