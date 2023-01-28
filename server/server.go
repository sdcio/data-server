package server

import (
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/datastore"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/schema"
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
	//

}

// type ServerConfig struct {
// 	Address string
// 	Secure  bool
// 	//
// 	Schemas    []*config.SchemaConfig
// 	Datastores []*config.DatastoreConfig
// }

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
			panic(err)
		}
		s.schemas[sc.UniqueName()] = sc
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
	fmt.Fprintf(os.Stderr, "running server on %s\n", s.config.GRPCServer.Address)

	err = s.srv.Serve(l)
	if err != nil {
		return err
	}

	return nil
}
