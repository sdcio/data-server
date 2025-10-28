// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip" // Install the gzip compressor
	"google.golang.org/grpc/status"

	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore"
	"github.com/sdcio/data-server/pkg/schema"
)

const (
	schemaServerConnectRetry = 10 * time.Second
)

type Server struct {
	config *config.Config
	ready  bool

	ctx context.Context
	cfn context.CancelFunc

	srv *grpc.Server
	sdcpb.UnimplementedDataServerServer
	sdcpb.UnimplementedSchemaServerServer

	router *mux.Router
	reg    *prometheus.Registry

	datastores *DatastoreMap

	schemaClient schema.Client
	cacheClient  cache.Client

	gnmiOpts []grpc.DialOption
}

type DatastoreMap struct {
	md         *sync.RWMutex
	datastores map[string]*datastore.Datastore // datastore group with sbi
}

func NewDatastoreMap() *DatastoreMap {
	return &DatastoreMap{
		md:         &sync.RWMutex{},
		datastores: map[string]*datastore.Datastore{},
	}
}
func (d *DatastoreMap) StopAll() {
	for _, ds := range d.datastores {
		ds.Stop()
	}
}

func (d *DatastoreMap) AddDatastore(ds *datastore.Datastore) error {
	d.md.Lock()
	defer d.md.Unlock()
	if existingDs, _ := d.getDataStore(ds.Name()); existingDs != nil {
		return fmt.Errorf("datastore %s already exists", ds.Name())
	}

	d.datastores[ds.Name()] = ds
	return nil
}

func (d *DatastoreMap) DeleteDatastore(ctx context.Context, name string) error {
	d.md.Lock()
	defer d.md.Unlock()
	ds, err := d.getDataStore(name)
	if err != nil {
		return err
	}
	ds.Delete(ctx)
	delete(d.datastores, name)
	return nil
}

func (d *DatastoreMap) GetDataStore(name string) (*datastore.Datastore, error) {
	d.md.RLock()
	defer d.md.RUnlock()
	return d.getDataStore(name)
}

func (d *DatastoreMap) GetDatastoreAll() []*datastore.Datastore {
	d.md.RLock()
	defer d.md.RUnlock()
	result := make([]*datastore.Datastore, 0, len(d.datastores))
	for _, x := range d.datastores {
		result = append(result, x)
	}
	return result
}

// getDataStore expects that the mutex d.md is already held
func (d *DatastoreMap) getDataStore(name string) (*datastore.Datastore, error) {
	ds, exists := d.datastores[name]
	if !exists {
		return nil, fmt.Errorf("unknown datastore %s", name)
	}
	return ds, nil
}

func New(ctx context.Context, c *config.Config) (*Server, error) {
	ctx, cancel := context.WithCancel(ctx)
	var s = &Server{
		config: c,
		ctx:    ctx,
		cfn:    cancel,

		datastores: NewDatastoreMap(),

		router:   mux.NewRouter(),
		reg:      prometheus.NewRegistry(),
		gnmiOpts: make([]grpc.DialOption, 0, 2),
	}

	// gRPC server options
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(c.GRPCServer.MaxRecvMsgSize),
	}
	// unary interceptors
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		s.readyInterceptor,
		s.timeoutInterceptor,
		contextLoggingInterceptor(s.ctx),
	}

	streamInterceptors := []grpc.StreamServerInterceptor{
		contextLoggingServerStreamInterceptor(s.ctx),
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
		streamInterceptors = append(streamInterceptors,
			grpcMetrics.StreamServerInterceptor(),
		)

		unaryInterceptors = append(unaryInterceptors, grpcMetrics.UnaryServerInterceptor())
		s.reg.MustRegister(grpcMetrics)
	}

	opts = append(opts,
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryInterceptors...)),
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(streamInterceptors...)),
	)

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
	log := logf.FromContext(ctx).WithValues("grpc-server-address", s.config.GRPCServer.Address)
	ctx = logf.IntoContext(ctx, log)
	l, err := net.Listen("tcp", s.config.GRPCServer.Address)
	if err != nil {
		return err
	}

	if s.config.Prometheus != nil {
		go s.ServeHTTP(ctx)
	}

	go s.startDataServer(ctx)

	log.Info("starting server")
	err = s.srv.Serve(l)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) ServeHTTP(ctx context.Context) {
	log := logf.FromContext(ctx)
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
		log.Error(err, "HTTP server stopped")
	}
}

func (s *Server) Stop() {
	s.srv.Stop()
	s.datastores.StopAll()
	s.cfn()
}

func (s *Server) startDataServer(ctx context.Context) {
	log := logf.FromContext(ctx)
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
	s.ready = true
	log.Info("data-server ready")
}

func (s *Server) createInitialDatastores(ctx context.Context) {
	log := logf.FromContext(ctx)
	numConfiguredDS := len(s.config.Datastores)
	if numConfiguredDS == 0 {
		return
	}
	wg := new(sync.WaitGroup)
	wg.Add(numConfiguredDS)

	for _, dsCfg := range s.config.Datastores {
		log.V(logf.VDebug).Info("creating datastore", "datastore-name", dsCfg.Name)
		dsCfg.Validation = s.config.Validation.DeepCopy()
		go func(dsCfg *config.DatastoreConfig) {
			defer wg.Done()
			// TODO: propagate error
			ds, err := datastore.New(ctx, dsCfg, s.schemaClient, s.cacheClient, s.gnmiOpts...)
			if err != nil {
				log.Error(err, "failed to create datastore")
			}
			err = s.datastores.AddDatastore(ds)
			if err != nil {
				log.Error(err, "failed to create datastore")
			}
		}(dsCfg)
	}
	wg.Wait()
	log.Info("configured datastores initialized")
}

func (s *Server) timeoutInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	ctx, cfn := context.WithTimeout(ctx, s.config.GRPCServer.RPCTimeout)
	defer cfn()
	return handler(ctx, req)
}

func (s *Server) readyInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	if !s.ready {
		return nil, status.Error(codes.Unavailable, "not ready")
	}
	return handler(ctx, req)
}

func contextLoggingInterceptor(logCtx context.Context) func(context.Context, interface{}, *grpc.UnaryServerInfo, grpc.UnaryHandler) (resp interface{}, err error) {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		uuidString := uuid.New().String()
		log := logf.FromContext(logCtx).WithValues("grpc-request-uuid", uuidString)
		ctx = logf.IntoContext(ctx, log)

		return handler(ctx, req)
	}
}

func contextLoggingServerStreamInterceptor(ctx context.Context) func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		uuidString := uuid.New().String()
		log := logf.FromContext(ctx).WithValues("grpc-request-uuid", uuidString)
		wss := grpc_middleware.WrapServerStream(ss)
		wss.WrappedContext = logf.IntoContext(wss.WrappedContext, log)

		return handler(srv, wss)
	}

}
