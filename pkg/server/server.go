package server

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip" // Install the gzip compressor

	"github.com/iptecharch/data-server/pkg/cache"
	"github.com/iptecharch/data-server/pkg/config"
	"github.com/iptecharch/data-server/pkg/datastore"
	"github.com/iptecharch/data-server/pkg/schema"
)

const (
	schemaServerConnectRetry = 10 * time.Second
)

type Server struct {
	config *config.Config

	ctx context.Context
	cfn context.CancelFunc

	md         *sync.RWMutex
	datastores map[string]*datastore.Datastore // datastore group with sbi

	srv *grpc.Server
	sdcpb.UnimplementedDataServerServer
	sdcpb.UnimplementedSchemaServerServer

	router *mux.Router
	reg    *prometheus.Registry

	// remoteSchemaClient sdcpb.SchemaServerClient

	schemaClient schema.Client
	cacheClient  cache.Client

	gnmiOpts []grpc.DialOption
}

func New(ctx context.Context, c *config.Config) (*Server, error) {
	ctx, cancel := context.WithCancel(ctx)
	var s = &Server{
		config: c,
		ctx:    ctx,
		cfn:    cancel,

		md:         &sync.RWMutex{},
		datastores: make(map[string]*datastore.Datastore),

		router:   mux.NewRouter(),
		reg:      prometheus.NewRegistry(),
		gnmiOpts: make([]grpc.DialOption, 0, 2),
	}

	// gRPC server options
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(c.GRPCServer.MaxRecvMsgSize),
	}

	if c.Prometheus != nil {
		// add gRPC client interceptors for gNMI
		grpcClientMetrics := grpc_prometheus.NewClientMetrics()
		s.gnmiOpts = append(s.gnmiOpts,
			grpc.WithUnaryInterceptor(grpcClientMetrics.UnaryClientInterceptor()),
			grpc.WithStreamInterceptor(grpcClientMetrics.StreamClientInterceptor()),
		)
		s.reg.MustRegister(grpcClientMetrics)

		// add gRPC server interceptors for the Schema/Data server
		grpcMetrics := grpc_prometheus.NewServerMetrics()
		opts = append(opts,
			grpc.StreamInterceptor(grpcMetrics.StreamServerInterceptor()),
		)
		unaryInterceptors := []grpc.UnaryServerInterceptor{
			func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
				ctx, cfn := context.WithTimeout(ctx, c.GRPCServer.RPCTimeout)
				defer cfn()
				return handler(ctx, req)
			},
		}
		unaryInterceptors = append(unaryInterceptors, grpcMetrics.UnaryServerInterceptor())
		opts = append(opts, grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryInterceptors...)))
		s.reg.MustRegister(grpcMetrics)
	}

	if c.GRPCServer.TLS != nil {
		tlsCfg, err := c.GRPCServer.TLS.NewConfig(ctx)
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsCfg)))
	}

	s.srv = grpc.NewServer(opts...)

	// register Data server gRPC Methods
	sdcpb.RegisterDataServerServer(s.srv, s)

	// register Schema server gRPC Methods
	if s.config.GRPCServer.SchemaServer != nil && s.config.GRPCServer.SchemaServer.Enabled {
		sdcpb.RegisterSchemaServerServer(s.srv, s)
	}

	return s, nil
}

func (s *Server) Serve(ctx context.Context) error {
	l, err := net.Listen("tcp", s.config.GRPCServer.Address)
	if err != nil {
		return err
	}

	if s.config.Prometheus != nil {
		go s.ServeHTTP()
	}

	go s.startDataServer(ctx)

	log.Infof("starting server on %s", s.config.GRPCServer.Address)
	err = s.srv.Serve(l)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) ServeHTTP() {
	s.router.Handle("/metrics", promhttp.HandlerFor(s.reg, promhttp.HandlerOpts{}))
	s.reg.MustRegister(collectors.NewGoCollector())
	s.reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	srv := &http.Server{
		Addr:         s.config.Prometheus.Address,
		Handler:      s.router,
		ReadTimeout:  time.Minute,
		WriteTimeout: time.Minute,
	}
	err := srv.ListenAndServe()
	if err != nil {
		log.Errorf("HTTP server stopped: %v", err)
	}
}

func (s *Server) Stop() {
	s.srv.Stop()
	for _, ds := range s.datastores {
		ds.Stop()
	}
	s.cfn()
}

func (s *Server) startDataServer(ctx context.Context) {
	wg := new(sync.WaitGroup)
	wg.Add(2)

	// create schemaClient, local or remote, based on config
	go func() {
		defer wg.Done()
		s.createSchemaClient(ctx)
	}()

	// create cacheClient, local or remote, based on config
	go func() {
		defer wg.Done()
		s.createCacheClient(ctx)
	}()

	wg.Wait()
	// init datastores
	s.createInitialDatastores(ctx)
	log.Infof("ready...")
}

func (s *Server) createInitialDatastores(ctx context.Context) {
	numConfiguredDS := len(s.config.Datastores)
	if numConfiguredDS == 0 {
		return
	}
	wg := new(sync.WaitGroup)
	wg.Add(numConfiguredDS)

	for _, dsCfg := range s.config.Datastores {
		log.Debugf("creating datastore %s", dsCfg.Name)
		go func(dsCfg *config.DatastoreConfig) {
			defer wg.Done()
			ds := datastore.New(ctx, dsCfg, s.schemaClient, s.cacheClient, s.gnmiOpts...)
			s.md.Lock()
			defer s.md.Unlock()
			s.datastores[dsCfg.Name] = ds
		}(dsCfg)
	}
	wg.Wait()
	log.Infof("configured datastores initialized")
}
