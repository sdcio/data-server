package server

import (
	"net"
	"sync"

	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/datastore"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/schema"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type Server struct {
	config *config.Config

	ms      *sync.RWMutex
	schemas map[string]*schema.Schema

	md         *sync.RWMutex
	datastores map[string]*datastore.Datastore // datastore group with sbi

	srv *grpc.Server
	schemapb.UnimplementedSchemaServerServer
	schemapb.UnimplementedDataServerServer

	schemaClient schemapb.SchemaServerClient
}

func NewServer(c *config.Config) (*Server, error) {
	var s = &Server{
		config: c,

		ms:      &sync.RWMutex{},
		schemas: make(map[string]*schema.Schema, len(c.Schemas)),

		md:         &sync.RWMutex{},
		datastores: make(map[string]*datastore.Datastore),
	}

	for _, sCfg := range c.Schemas {
		sc, err := schema.NewSchema(sCfg)
		if err != nil {
			return nil, err
		}
		s.schemas[sc.UniqueName()] = sc
	}
	for _, dsCfg := range c.Datastores {
		ds := datastore.New(dsCfg, c.SchemaServer)
		s.datastores[dsCfg.Name] = ds
	}
	opts := []grpc.ServerOption{}
	// todo: options
	s.srv = grpc.NewServer(opts...)
	schemapb.RegisterSchemaServerServer(s.srv, s)
	schemapb.RegisterDataServerServer(s.srv, s)
	return s, nil
}

func (s *Server) Serve() error {
	l, err := net.Listen("tcp", s.config.GRPCServer.Address)
	if err != nil {
		return err
	}
	log.Infof("running server on %s\n", s.config.GRPCServer.Address)

	err = s.srv.Serve(l)
	if err != nil {
		return err
	}

	return nil
}
