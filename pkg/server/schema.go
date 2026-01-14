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
	"strings"
	"sync"
	"time"

	"github.com/sdcio/data-server/pkg/schema"
	"github.com/sdcio/data-server/pkg/utils"
	logf "github.com/sdcio/logger"
	schemaConfig "github.com/sdcio/schema-server/pkg/config"
	schemaServerSchema "github.com/sdcio/schema-server/pkg/schema"
	schemaStore "github.com/sdcio/schema-server/pkg/store"
	schemaMemoryStore "github.com/sdcio/schema-server/pkg/store/memstore"
	schemaPersistentStore "github.com/sdcio/schema-server/pkg/store/persiststore"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
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
	log := logf.FromContext(ctx)
	var store schemaStore.Store
	switch s.config.SchemaStore.Type {
	case schemaConfig.StoreTypeMemory:
		store = schemaMemoryStore.New()
	case schemaConfig.StoreTypePersistent:
		var err error
		store, err = schemaPersistentStore.New(ctx, s.config.SchemaStore.Path, s.config.SchemaStore.Cache, s.config.SchemaStore.ReadOnly)
		if err != nil {
			log.Error(err, "failed to create a persistent schema store")
			os.Exit(1)
		}
	default:
		log.Error(nil, "unknown schema store type", "type", s.config.SchemaStore.Type)
		os.Exit(1)
	}
	numSchemas := len(s.config.SchemaStore.Schemas)
	log.Info("parsing schema(s)...", "numschemas", numSchemas)

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
			log = log.WithValues("schema", sck)
			if store.HasSchema(sck) {
				log.Info("schema already exists in the store, not reloading it")
				return
			}
			sc, err := schemaServerSchema.NewSchema(sCfg)
			if err != nil {
				log.Error(err, "schema parsing failed")
				return
			}
			err = store.AddSchema(sc)
			if err != nil {
				log.Error(err, "failed to add schema to the store")
				return
			}
		}(sCfg, store)
	}
	wg.Wait()
	s.schemaClient = schema.NewLocalClient(store)
}

func (s *Server) createRemoteSchemaClient(ctx context.Context) {
	log := logf.FromContext(ctx)
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
			log.Error(err, "failed to read schema server TLS config")
			time.Sleep(time.Second)
			goto SCHEMA_CONNECT
		}
		opts = append(opts,
			grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
		)
	}

	log = log.WithValues("schema-server-address", s.config.SchemaServer.Address)
	ctx = logf.IntoContext(ctx, log)

	dialCtx, cancel := context.WithTimeout(ctx, schemaServerConnectRetry)
	defer cancel()
	cc, err := grpc.DialContext(dialCtx, s.config.SchemaServer.Address, opts...)
	if err != nil {
		log.Error(err, "failed to connect to schema server")
		time.Sleep(time.Second)
		goto SCHEMA_CONNECT
	}
	log.Info("connected to schema server")
	s.schemaClient = schema.NewRemoteClient(cc, s.config.SchemaServer.Cache)
}

func (s *Server) GetSchema(ctx context.Context, req *sdcpb.GetSchemaRequest) (*sdcpb.GetSchemaResponse, error) {
	log := logf.FromContext(ctx).WithName("GetSchema")
	log = log.WithValues(
		"schema-name", req.GetSchema().GetName(),
		"schema-vendor", req.GetSchema().GetVendor(),
		"schema-version", req.GetSchema().GetVersion(),
	)
	ctx = logf.IntoContext(ctx, log)

	log.Info("GetSchema",
		"schema-path", req.GetPath().String(),
		"schema-validate-keys", req.GetValidateKeys(),
		"schema-with-description", req.GetWithDescription(),
	)
	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))
	return s.schemaClient.GetSchema(ctx, req)
}

func (s *Server) ListSchema(ctx context.Context, req *sdcpb.ListSchemaRequest) (*sdcpb.ListSchemaResponse, error) {
	log := logf.FromContext(ctx).WithName("ListSchema")
	ctx = logf.IntoContext(ctx, log)

	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))
	return s.schemaClient.ListSchema(ctx, req)
}

func (s *Server) GetSchemaDetails(ctx context.Context, req *sdcpb.GetSchemaDetailsRequest) (*sdcpb.GetSchemaDetailsResponse, error) {
	log := logf.FromContext(ctx).WithName("GetSchemaDetails")
	log = log.WithValues(
		"schema-name", req.GetSchema().GetName(),
		"schema-vendor", req.GetSchema().GetVendor(),
		"schema-version", req.GetSchema().GetVersion(),
	)
	ctx = logf.IntoContext(ctx, log)

	log.Info("GetSchemaDetails")
	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))
	return s.schemaClient.GetSchemaDetails(ctx, req)
}

func (s *Server) CreateSchema(ctx context.Context, req *sdcpb.CreateSchemaRequest) (*sdcpb.CreateSchemaResponse, error) {
	log := logf.FromContext(ctx).WithName("CreateSchema")
	log = log.WithValues(
		"schema-name", req.GetSchema().GetName(),
		"schema-vendor", req.GetSchema().GetVendor(),
		"schema-version", req.GetSchema().GetVersion(),
	)
	ctx = logf.IntoContext(ctx, log)

	log.Info("CreateSchema")
	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))
	return s.schemaClient.CreateSchema(ctx, req)
}

func (s *Server) ReloadSchema(ctx context.Context, req *sdcpb.ReloadSchemaRequest) (*sdcpb.ReloadSchemaResponse, error) {
	log := logf.FromContext(ctx).WithName("ReloadSchema")
	log = log.WithValues(
		"schema-name", req.GetSchema().GetName(),
		"schema-vendor", req.GetSchema().GetVendor(),
		"schema-version", req.GetSchema().GetVersion(),
	)
	ctx = logf.IntoContext(ctx, log)

	log.Info("ReloadSchema")
	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))
	return s.schemaClient.ReloadSchema(ctx, req)
}

func (s *Server) DeleteSchema(ctx context.Context, req *sdcpb.DeleteSchemaRequest) (*sdcpb.DeleteSchemaResponse, error) {
	log := logf.FromContext(ctx).WithName("DeleteSchema")
	log = log.WithValues(
		"schema-name", req.GetSchema().GetName(),
		"schema-vendor", req.GetSchema().GetVendor(),
		"schema-version", req.GetSchema().GetVersion(),
	)
	ctx = logf.IntoContext(ctx, log)

	log.Info("DeleteSchema")
	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))
	return s.schemaClient.DeleteSchema(ctx, req)
}

func (s *Server) ToPath(ctx context.Context, req *sdcpb.ToPathRequest) (*sdcpb.ToPathResponse, error) {
	log := logf.FromContext(ctx).WithName("ToPath")
	log = log.WithValues(
		"schema-name", req.GetSchema().GetName(),
		"schema-vendor", req.GetSchema().GetVendor(),
		"schema-version", req.GetSchema().GetVersion(),
	)
	ctx = logf.IntoContext(ctx, log)

	log.Info("ToPath",
		"path-elements", strings.Join(req.GetPathElement(), ","),
	)
	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))
	return s.schemaClient.ToPath(ctx, req)
}

func (s *Server) ExpandPath(ctx context.Context, req *sdcpb.ExpandPathRequest) (*sdcpb.ExpandPathResponse, error) {
	log := logf.FromContext(ctx).WithName("ExpandPath")
	log = log.WithValues(
		"schema-name", req.GetSchema().GetName(),
		"schema-vendor", req.GetSchema().GetVendor(),
		"schema-version", req.GetSchema().GetVersion(),
	)
	ctx = logf.IntoContext(ctx, log)

	log.Info("ExpandPath",
		"schema-path", req.GetPath().String(),
		"schema-datatype", req.GetDataType().String(),
		"schema-is-xpath", req.GetXpath(),
	)
	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))
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
	log := logf.FromContext(ctx).WithName("GetSchemaElements")
	log = log.WithValues(
		"schema-name", req.GetSchema().GetName(),
		"schema-vendor", req.GetSchema().GetVendor(),
		"schema-version", req.GetSchema().GetVersion(),
	)
	ctx = logf.IntoContext(ctx, log)

	log.Info("GetSchemaElements",
		"schema-path", req.GetPath().String(),
		"schema-validate-keys", req.GetValidateKeys(),
		"schema-with-description", req.GetWithDescription(),
	)
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
