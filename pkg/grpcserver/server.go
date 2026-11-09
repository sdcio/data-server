package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/core"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/status"
)

// Server serves DataServer and SchemaServer RPCs against a Core.
type Server struct {
	cfg  *config.Config
	core *core.Core

	ready atomic.Bool

	ctx context.Context
	cfn context.CancelFunc

	srv *grpc.Server
	sdcpb.UnimplementedDataServerServer
	sdcpb.UnimplementedSchemaServerServer

	router *mux.Router
	reg    *prometheus.Registry
}

// New constructs a Core (with gNMI Prometheus client interceptors when
// configured) and a gRPC Server sharing the same configuration.
func New(ctx context.Context, cfg *config.Config) (*core.Core, *Server, error) {
	ctx, cancel := context.WithCancel(ctx)
	s := &Server{
		cfg:    cfg,
		ctx:    ctx,
		cfn:    cancel,
		router: mux.NewRouter(),
		reg:    prometheus.NewRegistry(),
	}

	opts := []grpc.ServerOption{grpc.MaxRecvMsgSize(cfg.GRPCServer.MaxRecvMsgSize)}
	var dialOpts []grpc.DialOption

	unaryInterceptors := []grpc.UnaryServerInterceptor{s.readyInterceptor, s.timeoutInterceptor, contextLoggingInterceptor(ctx)}
	streamInterceptors := []grpc.StreamServerInterceptor{contextLoggingStreamInterceptor(ctx)}

	if cfg.Prometheus != nil {
		clientMetrics := grpc_prometheus.NewClientMetrics()
		dialOpts = append(dialOpts,
			grpc.WithUnaryInterceptor(clientMetrics.UnaryClientInterceptor()),
			grpc.WithStreamInterceptor(clientMetrics.StreamClientInterceptor()),
		)
		s.reg.MustRegister(clientMetrics)

		serverMetrics := grpc_prometheus.NewServerMetrics()
		unaryInterceptors = append(unaryInterceptors, serverMetrics.UnaryServerInterceptor())
		streamInterceptors = append(streamInterceptors, serverMetrics.StreamServerInterceptor())
		s.reg.MustRegister(serverMetrics)
	}

	opts = append(opts,
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryInterceptors...)),
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(streamInterceptors...)),
	)

	if cfg.GRPCServer.TLS != nil {
		tlsCfg, err := cfg.GRPCServer.TLS.NewConfig(ctx)
		if err != nil {
			cancel()
			return nil, nil, err
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsCfg)))
	}

	s.srv = grpc.NewServer(opts...)
	sdcpb.RegisterDataServerServer(s.srv, s)
	if cfg.GRPCServer.SchemaServer != nil && cfg.GRPCServer.SchemaServer.Enabled {
		sdcpb.RegisterSchemaServerServer(s.srv, s)
	}

	c, err := core.New(ctx, cfg, core.WithDialOptions(dialOpts...))
	if err != nil {
		cancel()
		return nil, nil, err
	}
	s.core = c
	return c, s, nil
}

// Run listens, marks ready when the core is up, then blocks in Serve until stop
// or error. The listener is opened before the core is ready; unary RPCs return
// Unavailable until WaitReady succeeds. Listen failures are retried until ctx ends.
func (s *Server) Run(ctx context.Context) error {
	log := logf.FromContext(ctx).WithName("grpc")

	if s.cfg.Prometheus != nil {
		go s.serveMetrics(ctx)
	}

	go func() {
		if err := s.core.WaitReady(ctx); err != nil {
			log.Error(err, "core did not become ready")
			return
		}
		s.ready.Store(true)
		log.Info("gRPC adapter ready")
	}()

	go func() {
		<-ctx.Done()
		s.Stop()
	}()

	for ctx.Err() == nil {
		l, err := net.Listen("tcp", s.cfg.GRPCServer.Address)
		if err != nil {
			log.Error(err, "listen failed, retrying")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				continue
			}
		}
		log.Info("starting gRPC server", "address", s.cfg.GRPCServer.Address)
		err = s.srv.Serve(l)
		if ctx.Err() != nil || errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return err
	}
	return ctx.Err()
}

func (s *Server) serveMetrics(ctx context.Context) {
	log := logf.FromContext(ctx)
	s.router.Handle("/metrics", promhttp.HandlerFor(s.reg, promhttp.HandlerOpts{}))
	s.reg.MustRegister(collectors.NewGoCollector())
	s.reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	srv := &http.Server{
		Addr:         s.cfg.Prometheus.Address,
		Handler:      s.router,
		ReadTimeout:  time.Minute,
		WriteTimeout: time.Minute,
	}
	if err := srv.ListenAndServe(); err != nil && ctx.Err() == nil {
		log.Error(err, "metrics server stopped")
	}
}

// Stop stops the gRPC server.
func (s *Server) Stop() {
	if s.srv != nil {
		s.srv.Stop()
	}
	s.cfn()
}

func (s *Server) readyInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if !s.ready.Load() {
		return nil, status.Error(codes.Unavailable, "not ready")
	}
	return handler(ctx, req)
}

func (s *Server) timeoutInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.GRPCServer.RPCTimeout)
	defer cancel()
	return handler(ctx, req)
}

func contextLoggingInterceptor(logCtx context.Context) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		uuidString := uuid.New().String()
		log := logf.FromContext(logCtx).WithValues("grpc-request-uuid", uuidString)
		ctx = logf.IntoContext(ctx, log)
		return handler(ctx, req)
	}
}

func contextLoggingStreamInterceptor(logCtx context.Context) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		uuidString := uuid.New().String()
		log := logf.FromContext(logCtx).WithValues("grpc-request-uuid", uuidString)
		wss := grpc_middleware.WrapServerStream(ss)
		wss.WrappedContext = logf.IntoContext(wss.WrappedContext, log)
		return handler(srv, wss)
	}
}

func transactionMissingName() error {
	return status.Error(codes.InvalidArgument, "missing datastore name")
}

func schemaClientOrError(s *Server) error {
	if s.core.SchemaClient() == nil {
		return status.Error(codes.Unavailable, "not ready")
	}
	return nil
}

func unexpectedSchemaError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("schema call: %w", err)
}
