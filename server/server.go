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
	"github.com/iptecharch/data-server/cache"
	"github.com/iptecharch/data-server/config"
	"github.com/iptecharch/data-server/datastore"
	"github.com/iptecharch/data-server/schema"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip" // Install the gzip compressor
)

const (
	schemaServerConnectRetry = 10 * time.Second
)

type Server struct {
	config *config.Config

	cfn context.CancelFunc

	md         *sync.RWMutex
	datastores map[string]*datastore.Datastore // datastore group with sbi

	srv *grpc.Server
	schemapb.UnimplementedDataServerServer
	schemapb.UnimplementedSchemaServerServer

	router *mux.Router
	reg    *prometheus.Registry

	// remoteSchemaClient schemapb.SchemaServerClient

	schemaClient schema.Client
	cacheClient  cache.Client

	gnmiOpts []grpc.DialOption
}

func New(ctx context.Context, c *config.Config) (*Server, error) {
	ctx, cancel := context.WithCancel(ctx)
	var s = &Server{
		config: c,
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
	schemapb.RegisterDataServerServer(s.srv, s)

	// register Schema server gRPC Methods
	if s.config.GRPCServer.SchemaServer != nil && s.config.GRPCServer.SchemaServer.Enabled {
		schemapb.RegisterSchemaServerServer(s.srv, s)
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
	// create schemaClient, local or remote, based on config
	s.createSchemaClient(ctx)

	// create cacheClient, local or remote, based on config
	s.createCacheClient(ctx)

	// init datastores
	s.createDatastores()
	log.Infof("ready...")
}

func (s *Server) createDatastores() {
	wg := new(sync.WaitGroup)
	wg.Add(len(s.config.Datastores))
	for _, dsCfg := range s.config.Datastores {
		log.Debugf("creating datastore %s", dsCfg.Name)
		go func(dsCfg *config.DatastoreConfig) {
			defer wg.Done()
			defer s.md.Unlock()
			s.md.Lock()
			s.datastores[dsCfg.Name] = datastore.New(dsCfg, s.schemaClient, s.cacheClient, s.gnmiOpts...)
		}(dsCfg)
	}
	wg.Wait()
	log.Infof("configured datastores initialized")
}
