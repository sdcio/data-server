package grpcserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"time"

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
	_ "google.golang.org/grpc/encoding/gzip" // installs the gzip compressor
	"google.golang.org/grpc/status"
)

const (
	retryMin = time.Second
	retryMax = 10 * time.Second
)

// Server exposes a core.Core as the sdcpb DataServer and, when enabled in the
// config, SchemaServer services. The caller starts the core (core.Start) and
// then calls Run, which blocks until ctx ends or serving fails for good. Stop
// is idempotent and does not stop the core.
type Server struct {
	cfg  *config.Config
	core *core.Core

	// ready flips once the core is ready; unary RPCs fail with Unavailable before.
	ready atomic.Bool

	ctx    context.Context
	cancel context.CancelFunc

	srv *grpc.Server
	sdcpb.UnimplementedDataServerServer
	sdcpb.UnimplementedSchemaServerServer

	router *mux.Router
	reg    *prometheus.Registry

	// listen opens the listener; tests replace it with an in-memory one.
	listen func() (net.Listener, error)
}

// New builds the core and the gRPC server around it. When Prometheus is
// configured the core's gNMI client connections carry client metrics. The
// core's lifetime follows ctx; the server's own resources follow Stop.
func New(ctx context.Context, cfg *config.Config) (*core.Core, *Server, error) {
	srvCtx, cancel := context.WithCancel(ctx)
	s := &Server{
		cfg:    cfg,
		ctx:    srvCtx,
		cancel: cancel,
		router: mux.NewRouter(),
		reg:    prometheus.NewRegistry(),
		listen: func() (net.Listener, error) { return net.Listen("tcp", cfg.GRPCServer.Address) },
	}

	var dialOpts []grpc.DialOption
	unary := []grpc.UnaryServerInterceptor{s.readyInterceptor, s.timeoutInterceptor, loggingUnaryInterceptor(srvCtx)}
	stream := []grpc.StreamServerInterceptor{loggingStreamInterceptor(srvCtx)}

	if cfg.Prometheus != nil {
		clientMetrics := grpc_prometheus.NewClientMetrics()
		dialOpts = append(dialOpts,
			grpc.WithUnaryInterceptor(clientMetrics.UnaryClientInterceptor()),
			grpc.WithStreamInterceptor(clientMetrics.StreamClientInterceptor()),
		)
		s.reg.MustRegister(clientMetrics)

		serverMetrics := grpc_prometheus.NewServerMetrics()
		unary = append(unary, serverMetrics.UnaryServerInterceptor())
		stream = append(stream, serverMetrics.StreamServerInterceptor())
		s.reg.MustRegister(serverMetrics)
	}

	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(cfg.GRPCServer.MaxRecvMsgSize),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unary...)),
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(stream...)),
	}
	if cfg.GRPCServer.TLS != nil {
		tlsCfg, err := cfg.GRPCServer.TLS.NewConfig(srvCtx)
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

// Run serves until ctx ends, Stop is called, or the core fails to become
// ready, which it returns. The listener opens immediately, before the core is
// ready. Listen and serve errors are retried with a growing delay.
func (s *Server) Run(ctx context.Context) error {
	log := logf.FromContext(ctx).WithName("grpc")

	if s.cfg.Prometheus != nil {
		go s.serveMetrics(ctx)
	}

	readyErr := make(chan error, 1)
	go func() {
		if err := s.core.WaitReady(ctx); err != nil {
			if ctx.Err() == nil {
				readyErr <- err
				s.srv.Stop()
			}
			return
		}
		s.ready.Store(true)
		log.Info("gRPC server ready")
	}()
	go func() {
		<-ctx.Done()
		s.srv.Stop()
	}()

	delay := retryMin
	for {
		l, err := s.listen()
		if err == nil {
			log.Info("starting gRPC server", "address", s.cfg.GRPCServer.Address)
			err = s.srv.Serve(l)
		}
		select {
		case rerr := <-readyErr:
			return rerr
		default:
		}
		if err == nil || errors.Is(err, grpc.ErrServerStopped) || ctx.Err() != nil {
			return nil
		}
		log.Error(err, "gRPC listen/serve failed, retrying", "retry-in", delay.String())
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
		delay = min(delay*2, retryMax)
	}
}

func (s *Server) serveMetrics(ctx context.Context) {
	s.router.Handle("/metrics", promhttp.HandlerFor(s.reg, promhttp.HandlerOpts{}))
	s.reg.MustRegister(collectors.NewGoCollector())
	s.reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	srv := &http.Server{
		Addr:         s.cfg.Prometheus.Address,
		Handler:      s.router,
		ReadTimeout:  time.Minute,
		WriteTimeout: time.Minute,
	}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logf.FromContext(ctx).Error(err, "HTTP server stopped")
	}
}

// Stop stops the gRPC server and releases its resources. The core keeps running.
func (s *Server) Stop() {
	s.srv.Stop()
	s.cancel()
}

func (s *Server) readyInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if !s.ready.Load() {
		return nil, status.Error(codes.Unavailable, "not ready")
	}
	return handler(ctx, req)
}

func (s *Server) timeoutInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.GRPCServer.RPCTimeout)
	defer cancel()
	return handler(ctx, req)
}
