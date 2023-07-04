package server

import (
	"context"
	"sync"
	"time"

	"github.com/iptecharch/data-server/schema"
	schemaConfig "github.com/iptecharch/schema-server/config"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	schemaStore "github.com/iptecharch/schema-server/schema"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func (s *Server) createSchemaClient(ctx context.Context) {
	switch {
	case s.config.Schemas != nil:
		// local schema store
		store := schemaStore.NewStore()
		numSchemas := len(s.config.Schemas)
		log.Infof("parsing %d schema(s)...", numSchemas)

		wg := new(sync.WaitGroup)
		wg.Add(numSchemas)
		for _, sCfg := range s.config.Schemas {
			go func(sCfg *schemaConfig.SchemaConfig) {
				defer wg.Done()
				sc, err := schemaStore.NewSchema(sCfg)
				if err != nil {
					log.Errorf("schema %s parsing failed: %v", sCfg.Name, err)
					return
				}
				store.AddSchema(sc)
			}(sCfg)
		}
		wg.Wait()
		s.schemaClient = schema.NewLocalClient(store)
	default:
		// remote schema store
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
		log.Infof("connected to schema server: %s", s.config.SchemaServer.Address)
		// s.remoteSchemaClient = schemapb.NewSchemaServerClient(cc)
		s.schemaClient = schema.NewRemoteClient(cc)
	}
}

func (s *Server) GetSchema(ctx context.Context, req *schemapb.GetSchemaRequest) (*schemapb.GetSchemaResponse, error) {
	log.Debugf("received GetSchemaRequest: %v", req)
	return s.schemaClient.GetSchema(ctx, req)
}

func (s *Server) ListSchema(ctx context.Context, req *schemapb.ListSchemaRequest) (*schemapb.ListSchemaResponse, error) {
	log.Debugf("received ListSchema: %v", req)
	return s.schemaClient.ListSchema(ctx, req)
}

func (s *Server) GetSchemaDetails(ctx context.Context, req *schemapb.GetSchemaDetailsRequest) (*schemapb.GetSchemaDetailsResponse, error) {
	log.Debugf("received GetSchemaDetails: %v", req)
	return s.schemaClient.GetSchemaDetails(ctx, req)
}

func (s *Server) CreateSchema(ctx context.Context, req *schemapb.CreateSchemaRequest) (*schemapb.CreateSchemaResponse, error) {
	log.Debugf("received CreateSchema: %v", req)
	return s.schemaClient.CreateSchema(ctx, req)
}

func (s *Server) ReloadSchema(ctx context.Context, req *schemapb.ReloadSchemaRequest) (*schemapb.ReloadSchemaResponse, error) {
	log.Debugf("received ReloadSchema: %v", req)
	return s.schemaClient.ReloadSchema(ctx, req)
}

func (s *Server) DeleteSchema(ctx context.Context, req *schemapb.DeleteSchemaRequest) (*schemapb.DeleteSchemaResponse, error) {
	log.Debugf("received DeleteSchema: %v", req)
	return s.schemaClient.DeleteSchema(ctx, req)
}

func (s *Server) ToPath(ctx context.Context, req *schemapb.ToPathRequest) (*schemapb.ToPathResponse, error) {
	log.Debugf("received ToPath: %v", req)
	return s.schemaClient.ToPath(ctx, req)
}

func (s *Server) ExpandPath(ctx context.Context, req *schemapb.ExpandPathRequest) (*schemapb.ExpandPathResponse, error) {
	log.Debugf("received ExpandPath: %v", req)
	return s.schemaClient.ExpandPath(ctx, req)
}

// BROKEN
func (s *Server) UploadSchema(stream schemapb.SchemaServer_UploadSchemaServer) error {
	schemaUploadClient, err := s.schemaClient.UploadSchema(stream.Context())
	if err != nil {
		return err
	}

	for {
		updloadFileReq, err := stream.Recv()
		if err != nil {
			return err
		}
		err = schemaUploadClient.Send(updloadFileReq)
		if err != nil {
			return err
		}
	}
}

func (s *Server) GetSchemaElements(req *schemapb.GetSchemaRequest, stream schemapb.SchemaServer_GetSchemaElementsServer) error {
	ctx := stream.Context()
	ch, err := s.schemaClient.GetSchemaElements(ctx, req)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sce, ok := <-ch:
			if !ok {
				return nil
			}
			err = stream.Send(&schemapb.GetSchemaResponse{
				Schema: sce,
			})
			if err != nil {
				return err
			}
		}
	}
}
