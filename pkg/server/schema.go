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
	"os"
	"sync"
	"time"

	schemaConfig "github.com/sdcio/schema-server/pkg/config"
	schemaServerSchema "github.com/sdcio/schema-server/pkg/schema"
	schemaStore "github.com/sdcio/schema-server/pkg/store"
	schemaMemoryStore "github.com/sdcio/schema-server/pkg/store/memstore"
	schemaPersistentStore "github.com/sdcio/schema-server/pkg/store/persiststore"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/sdcio/data-server/pkg/schema"
)

func (s *Server) createSchemaClient(ctx context.Context) {
	switch {
	case s.config.SchemaStore != nil:
		// local schema store
		s.createLocalSchemaStore(ctx)
	default:
		// remote schema store
		s.createRemoteSchemaClient(ctx)
	}
}

func (s *Server) createLocalSchemaStore(ctx context.Context) {
	var store schemaStore.Store
	switch s.config.SchemaStore.Type {
	case schemaConfig.StoreTypeMemory:
		store = schemaMemoryStore.New()
	case schemaConfig.StoreTypePersistent:
		var err error
		store, err = schemaPersistentStore.New(ctx, s.config.SchemaStore.Path, s.config.SchemaStore.Cache)
		if err != nil {
			log.Errorf("failed to create a persistent schema store: %v", err)
			os.Exit(1)
		}
	default:
		log.Errorf("unknown schema store type %s", s.config.SchemaStore.Type)
		os.Exit(1)
	}
	numSchemas := len(s.config.SchemaStore.Schemas)
	log.Infof("parsing %d schema(s)...", numSchemas)

	wg := new(sync.WaitGroup)
	wg.Add(numSchemas)
	for _, sCfg := range s.config.SchemaStore.Schemas {
		go func(sCfg *schemaConfig.SchemaConfig, store schemaStore.Store) {
			defer wg.Done()
			sck := schemaStore.SchemaKey{
				Name:    sCfg.Name,
				Vendor:  sCfg.Vendor,
				Version: sCfg.Version,
			}
			if store.HasSchema(sck) {
				log.Infof("schema %s already exists in the store: not reloading it...", sck)
				return
			}
			sc, err := schemaServerSchema.NewSchema(sCfg)
			if err != nil {
				log.Errorf("schema %s parsing failed: %v", sCfg.Name, err)
				return
			}
			store.AddSchema(sc)
		}(sCfg, store)
	}
	wg.Wait()
	s.schemaClient = schema.NewLocalClient(store)
}

func (s *Server) createRemoteSchemaClient(ctx context.Context) {
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
	s.schemaClient = schema.NewRemoteClient(cc, s.config.SchemaServer.Cache)
}

func (s *Server) GetSchema(ctx context.Context, req *sdcpb.GetSchemaRequest) (*sdcpb.GetSchemaResponse, error) {
	log.Debugf("received GetSchemaRequest: %v", req)
	return s.schemaClient.GetSchema(ctx, req)
}

func (s *Server) ListSchema(ctx context.Context, req *sdcpb.ListSchemaRequest) (*sdcpb.ListSchemaResponse, error) {
	log.Debugf("received ListSchema: %v", req)
	return s.schemaClient.ListSchema(ctx, req)
}

func (s *Server) GetSchemaDetails(ctx context.Context, req *sdcpb.GetSchemaDetailsRequest) (*sdcpb.GetSchemaDetailsResponse, error) {
	log.Debugf("received GetSchemaDetails: %v", req)
	return s.schemaClient.GetSchemaDetails(ctx, req)
}

func (s *Server) CreateSchema(ctx context.Context, req *sdcpb.CreateSchemaRequest) (*sdcpb.CreateSchemaResponse, error) {
	log.Debugf("received CreateSchema: %v", req)
	return s.schemaClient.CreateSchema(ctx, req)
}

func (s *Server) ReloadSchema(ctx context.Context, req *sdcpb.ReloadSchemaRequest) (*sdcpb.ReloadSchemaResponse, error) {
	log.Debugf("received ReloadSchema: %v", req)
	return s.schemaClient.ReloadSchema(ctx, req)
}

func (s *Server) DeleteSchema(ctx context.Context, req *sdcpb.DeleteSchemaRequest) (*sdcpb.DeleteSchemaResponse, error) {
	log.Debugf("received DeleteSchema: %v", req)
	return s.schemaClient.DeleteSchema(ctx, req)
}

func (s *Server) ToPath(ctx context.Context, req *sdcpb.ToPathRequest) (*sdcpb.ToPathResponse, error) {
	log.Debugf("received ToPath: %v", req)
	return s.schemaClient.ToPath(ctx, req)
}

func (s *Server) ExpandPath(ctx context.Context, req *sdcpb.ExpandPathRequest) (*sdcpb.ExpandPathResponse, error) {
	log.Debugf("received ExpandPath: %v", req)
	return s.schemaClient.ExpandPath(ctx, req)
}

// BROKEN
func (s *Server) UploadSchema(stream sdcpb.SchemaServer_UploadSchemaServer) error {
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

func (s *Server) GetSchemaElements(req *sdcpb.GetSchemaRequest, stream sdcpb.SchemaServer_GetSchemaElementsServer) error {
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
			err = stream.Send(&sdcpb.GetSchemaResponse{
				Schema: sce,
			})
			if err != nil {
				return err
			}
		}
	}
}
