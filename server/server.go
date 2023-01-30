package server

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/datastore"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/schema"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type Server struct {
	config *config.Config

	cfn context.CancelFunc

	ms      *sync.RWMutex
	schemas map[string]*schema.Schema

	md         *sync.RWMutex
	datastores map[string]*datastore.Datastore // datastore group with sbi

	srv *grpc.Server
	schemapb.UnimplementedSchemaServerServer
	schemapb.UnimplementedDataServerServer

	router *mux.Router
	reg    *prometheus.Registry
	// schemaClient schemapb.SchemaServerClient
}

func NewServer(c *config.Config) (*Server, error) {
	ctx, cancel := context.WithCancel(context.TODO())
	var s = &Server{
		config: c,
		cfn:    cancel,

		ms:      &sync.RWMutex{},
		schemas: make(map[string]*schema.Schema, len(c.Schemas)),

		md:         &sync.RWMutex{},
		datastores: make(map[string]*datastore.Datastore),

		router: mux.NewRouter(),
		reg:    prometheus.NewRegistry(),
	}

	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(c.GRPCServer.MaxRecvMsgSize),
	}
	if c.Prometheus != nil {
		grpcMetrics := grpc_prometheus.NewServerMetrics()
		opts = append(opts,
			grpc.StreamInterceptor(grpcMetrics.StreamServerInterceptor()),
			grpc.UnaryInterceptor(grpcMetrics.UnaryServerInterceptor()),
		)
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
	if c.GRPCServer.SchemaServer {
		for _, sCfg := range c.Schemas {
			sc, err := schema.NewSchema(sCfg)
			if err != nil {
				return nil, err
			}
			s.schemas[sc.UniqueName()] = sc
		}
		schemapb.RegisterSchemaServerServer(s.srv, s)
	}
	if c.GRPCServer.DataServer {
		for _, dsCfg := range c.Datastores {
			ds := datastore.New(dsCfg, c.SchemaServer)
			s.datastores[dsCfg.Name] = ds
		}
		schemapb.RegisterDataServerServer(s.srv, s)
	}
	return s, nil
}

func (s *Server) Serve() error {
	l, err := net.Listen("tcp", s.config.GRPCServer.Address)
	if err != nil {
		return err
	}
	log.Infof("running server on %s\n", s.config.GRPCServer.Address)
	if s.config.Prometheus != nil {
		go s.ServeHTTP()
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
