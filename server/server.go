package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/mux"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	cconfig "github.com/iptecharch/cache/config"
	"github.com/iptecharch/data-server/cache"
	"github.com/iptecharch/data-server/config"
	"github.com/iptecharch/data-server/datastore"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
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

	router *mux.Router
	reg    *prometheus.Registry

	remoteSchemaClient schemapb.SchemaServerClient
	cacheClient        cache.Client

	gnmiOpts []grpc.DialOption
}

func NewServer(c *config.Config) (*Server, error) {
	ctx, cancel := context.WithCancel(context.TODO())
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
	if c.GRPCServer.DataServer != nil && c.GRPCServer.DataServer.Enabled {
		schemapb.RegisterDataServerServer(s.srv, s)
	}
	return s, nil
}

func (s *Server) Serve(ctx context.Context) error {
	l, err := net.Listen("tcp", s.config.GRPCServer.Address)
	if err != nil {
		return err
	}
	log.Infof("running server on %s", s.config.GRPCServer.Address)
	if s.config.Prometheus != nil {
		go s.ServeHTTP()
	}
	if s.config.GRPCServer.DataServer != nil && s.config.GRPCServer.DataServer.Enabled {
		go s.startDataServer(ctx)
	}
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
	// create schemaClient
	s.CreateSchemaClient(ctx)

	// create cacheClient
	s.CreateCacheClient(ctx)

	// create datastores
	s.createDatastores()
}

func (s *Server) CreateSchemaClient(ctx context.Context) {
SCHEMA_CONNECT:
	opts := []grpc.DialOption{
		grpc.WithBlock(),
	}
	switch s.config.SchemaServer.TLS {
	case nil:
		opts = append(opts,
			grpc.WithTransportCredentials(
				insecure.NewCredentials(),
			))
	default:
		tlsCfg, err := s.config.SchemaServer.TLS.NewConfig(ctx)
		if err != nil {
			log.Errorf("failed to read schema server TLS config: %v", err)
			time.Sleep(time.Second)
			goto SCHEMA_CONNECT
		}
		opts = append(opts,
			grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
		)
	}

	dialCtx, cancel := context.WithTimeout(ctx, schemaServerConnectRetry)
	defer cancel()
	cc, err := grpc.DialContext(dialCtx, s.config.SchemaServer.Address, opts...)
	if err != nil {
		log.Errorf("failed to connect DS to schema server: %v", err)
		time.Sleep(time.Second)
		goto SCHEMA_CONNECT
	}
	s.remoteSchemaClient = schemapb.NewSchemaServerClient(cc)
}

func (s *Server) CreateCacheClient(ctx context.Context) {
START:
	var err error
	switch s.config.Cache.Type {
	default:
		fmt.Fprintf(os.Stderr, "unknown cache type: %s", s.config.Cache.Type)
		os.Exit(1)
	case "local":
		err = s.createLocalCacheClient(ctx)
		if err != nil {
			log.Errorf("failed to initialize a local cache client: %v", err)
			time.Sleep(time.Second)
			goto START
		}
		log.Infof("local cache created")
	case "remote":
		err = s.createRemoteCacheClient(ctx)
		if err != nil {
			log.Errorf("failed to initialize a remote cache client: %v", err)
			time.Sleep(time.Second)
			goto START
		}
		log.Infof("connected to remote cache")
	}
}

func (s *Server) createLocalCacheClient(ctx context.Context) error {
	var err error
	log.Infof("initializing local cache client")
	s.cacheClient, err = cache.NewLocalCache(&cconfig.CacheConfig{
		MaxCaches: -1,
		StoreType: s.config.Cache.StoreType,
		Dir:       s.config.Cache.Dir,
	})
	return err
}

func (s *Server) createRemoteCacheClient(ctx context.Context) error {
	log.Infof("initializing remote cache client")
	var err error
	s.cacheClient, err = cache.NewRemoteCache(ctx, s.config.Cache.Address)
	return err
}

func (s *Server) createDatastores() {
	wg := new(sync.WaitGroup)
	wg.Add(len(s.config.Datastores))
	for _, dsCfg := range s.config.Datastores {
		go func(dsCfg *config.DatastoreConfig) {
			defer wg.Done()
			ds := datastore.New(dsCfg, s.remoteSchemaClient, s.cacheClient, s.gnmiOpts...)
			s.md.Lock()
			s.datastores[dsCfg.Name] = ds
			s.md.Unlock()
		}(dsCfg)
	}
	wg.Wait()
}
