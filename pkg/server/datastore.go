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
	"math"
	"strings"
	"time"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore"
	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/utils"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// datastore
func (s *Server) ListDataStore(ctx context.Context, req *sdcpb.ListDataStoreRequest) (*sdcpb.ListDataStoreResponse, error) {
	log := logf.FromContext(ctx).WithName("ListDataStore")
	ctx = logf.IntoContext(ctx, log)

	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))

	datastores := s.datastores.GetDatastoreAll()
	rs := make([]*sdcpb.GetDataStoreResponse, 0, len(datastores))
	for _, ds := range datastores {
		r, err := s.datastoreToRsp(ctx, ds)
		if err != nil {
			return nil, err
		}
		rs = append(rs, r)
	}
	return &sdcpb.ListDataStoreResponse{
		Datastores: rs,
	}, nil
}

func (s *Server) GetDataStore(ctx context.Context, req *sdcpb.GetDataStoreRequest) (*sdcpb.GetDataStoreResponse, error) {
	log := logf.FromContext(ctx).WithName("GetDataStore")
	log = log.WithValues(
		"datastore-name", req.GetDatastoreName(),
	)
	ctx = logf.IntoContext(ctx, log)

	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))
	name := req.GetDatastoreName()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name attribute")
	}
	ds, err := s.datastores.GetDataStore(req.GetDatastoreName())
	if err != nil {
		return nil, err
	}
	return s.datastoreToRsp(ctx, ds)
}

func (s *Server) CreateDataStore(ctx context.Context, req *sdcpb.CreateDataStoreRequest) (*sdcpb.CreateDataStoreResponse, error) {
	log := logf.FromContext(ctx).WithName("CreateDataStore")
	log = log.WithValues(
		"datastore-name", req.GetDatastoreName(),
	)
	ctx = logf.IntoContext(ctx, log)

	log.Info("creating datastore",
		"datastore-schema", req.GetSchema(),
		"datastore-target", req.GetTarget(),
	)
	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))

	name := req.GetDatastoreName()
	lName := len(name)
	if lName == 0 {
		log.V(logf.VError).Info("missing datastore name attribute")
		return nil, status.Error(codes.InvalidArgument, "missing datastore name attribute")
	}
	if lName > math.MaxUint16 {
		log.V(logf.VError).Info("datastore name attribute too long")
		return nil, status.Error(codes.InvalidArgument, "datastore name attribute too long")
	}

	if _, err := s.datastores.GetDataStore(name); err == nil {
		log.V(logf.VError).Info("datastore already exists")
		return nil, status.Errorf(codes.InvalidArgument, "datastore %s already exists", name)
	}

	sbi := &config.SBI{
		Type:    req.GetTarget().GetType(),
		Port:    req.GetTarget().GetPort(),
		Address: req.GetTarget().GetAddress(),
	}

	switch strings.ToLower(req.GetTarget().GetType()) {
	case "netconf":
		commitDatastore := "candidate"
		switch req.GetTarget().GetNetconfOpts().GetCommitCandidate() {
		case sdcpb.CommitCandidate_COMMIT_CANDIDATE:
		case sdcpb.CommitCandidate_COMMIT_RUNNING:
			commitDatastore = "running"
		default:
			log.V(logf.VError).Info("unknown commitDatastore", "datastore", req.GetTarget().GetNetconfOpts().GetCommitCandidate())
			return nil, fmt.Errorf("unknown commitDatastore: %v", req.GetTarget().GetNetconfOpts().GetCommitCandidate())
		}
		sbi.NetconfOptions = &config.SBINetconfOptions{
			IncludeNS:              req.GetTarget().GetNetconfOpts().GetIncludeNs(),
			OperationWithNamespace: req.GetTarget().GetNetconfOpts().GetOperationWithNs(),
			UseOperationRemove:     req.GetTarget().GetNetconfOpts().GetUseOperationRemove(),
			CommitDatastore:        commitDatastore,
		}

	case "gnmi":
		sbi.GnmiOptions = &config.SBIGnmiOptions{
			Encoding: req.GetTarget().GetGnmiOpts().GetEncoding(),
		}
	default:
		log.V(logf.VError).Info("unknowm targetconnection protocol type", "type", req.GetTarget().GetType())
		return nil, fmt.Errorf("unknowm targetconnection protocol type %s", req.GetTarget().GetType())
	}

	if req.GetTarget().GetTls() != nil {
		sbi.TLS = &config.TLS{
			CA:         req.GetTarget().GetTls().GetCa(),
			Cert:       req.GetTarget().GetTls().GetCert(),
			Key:        req.GetTarget().GetTls().GetKey(),
			SkipVerify: req.GetTarget().GetTls().GetSkipVerify(),
		}
	}
	if req.GetTarget().GetCredentials() != nil {
		sbi.Credentials = &config.Creds{
			Username: req.GetTarget().GetCredentials().GetUsername(),
			Password: req.GetTarget().GetCredentials().GetPassword(),
			Token:    req.GetTarget().GetCredentials().GetToken(),
		}
	}

	dsConfig := &config.DatastoreConfig{
		Name: name,
		Schema: &config.SchemaConfig{
			Name:    req.GetSchema().GetName(),
			Vendor:  req.GetSchema().GetVendor(),
			Version: req.GetSchema().GetVersion(),
		},
		SBI:        sbi,
		Validation: s.config.Validation.DeepCopy(),
		Deviation:  s.config.Deviation.DeepCopy(),
	}
	if req.GetSync() != nil {
		dsConfig.Sync = &config.Sync{
			Validate:     req.GetSync().GetValidate(),
			Buffer:       req.GetSync().GetBuffer(),
			WriteWorkers: req.GetSync().GetWriteWorkers(),
			Config:       make([]*config.SyncProtocol, 0, len(req.GetSync().GetConfig())),
		}
		for _, pSync := range req.GetSync().GetConfig() {
			gnSyncConfig := &config.SyncProtocol{
				Protocol: pSync.GetTarget().GetType(),
				Name:     pSync.GetName(),
				Paths:    pSync.GetPath(),
				Interval: time.Duration(pSync.GetInterval()),
			}
			switch strings.ToLower(pSync.GetTarget().GetType()) {
			case "gnmi":
				gnSyncConfig.Mode = "on-change"
				switch pSync.GetMode() {
				case sdcpb.SyncMode_SM_ON_CHANGE:
				case sdcpb.SyncMode_SM_SAMPLE:
					gnSyncConfig.Mode = "sample"
				case sdcpb.SyncMode_SM_ONCE:
					gnSyncConfig.Mode = "once"
				case sdcpb.SyncMode_SM_GET:
					gnSyncConfig.Mode = "get"
				}
				gnSyncConfig.Encoding = pSync.GetTarget().GetGnmiOpts().GetEncoding()
			case "netconf":
			default:
				log.V(logf.VError).Info("unknown targetsync protocol type", "protocol", pSync.GetTarget().GetType())
				return nil, status.Errorf(codes.InvalidArgument, "unknown targetsync protocol: %q", pSync.GetTarget().GetType())
			}
			dsConfig.Sync.Config = append(dsConfig.Sync.Config, gnSyncConfig)
		}
	}
	err := dsConfig.ValidateSetDefaults()
	if err != nil {
		log.Error(err, "invalid datastore config")
		return nil, status.Errorf(codes.InvalidArgument, "invalid datastore config: %v", err)
	}
	ds, err := datastore.New(
		s.ctx,
		dsConfig,
		s.schemaClient,
		s.cacheClient,
		s.gnmiOpts...)
	if err != nil {
		log.Error(err, "failed creating new datastore")
		return nil, err
	}
	err = s.datastores.AddDatastore(ds)
	if err != nil {
		log.Error(err, "failed adding new datastore")
		return nil, err
	}
	log.Info("datastore created successfully")
	return &sdcpb.CreateDataStoreResponse{}, nil
}

func (s *Server) DeleteDataStore(ctx context.Context, req *sdcpb.DeleteDataStoreRequest) (*sdcpb.DeleteDataStoreResponse, error) {
	log := logf.FromContext(ctx).WithName("DeleteDataStore")
	ctx = logf.IntoContext(ctx, log)

	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))

	name := req.GetName()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name attribute")
	}

	ds, err := s.datastores.GetDataStore(name)
	if err != nil {
		return nil, err
	}

	err = ds.Stop(s.ctx)
	if err != nil {
		log.Error(err, "failed to stop datastore")
	}
	err = s.datastores.DeleteDatastore(ctx, name)
	if err != nil {
		log.Error(err, "failed to delete datastore")
		return nil, fmt.Errorf("failed to delete datastore: %w", err)
	}
	log.Info("deleted datastore")

	return &sdcpb.DeleteDataStoreResponse{}, nil
}

func (s *Server) WatchDeviations(req *sdcpb.WatchDeviationRequest, stream sdcpb.DataServer_WatchDeviationsServer) error {
	ctx := stream.Context()
	p, ok := peer.FromContext(ctx)
	var peerName string
	if ok {
		peerName = p.Addr.String()
	}
	log := logf.FromContext(ctx).WithName("WatchDeviations").WithValues("peer", peerName)
	ctx = logf.IntoContext(ctx, log)

	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))

	if !ok {
		return status.Errorf(codes.InvalidArgument, "missing peer info")
	}

	if req.GetName() == nil || len(req.GetName()) == 0 {
		return status.Errorf(codes.InvalidArgument, "missing datastore name")
	}
	if len(req.GetName()) > 1 {
		// although we have a slice in the req we support just a single datastore per request atm.
		return status.Errorf(codes.InvalidArgument, "only single datastore name allowed per request")
	}

	ds, err := s.datastores.GetDataStore(req.GetName()[0])
	if err != nil {
		log.Error(err, "failed to get datastore")
		return status.Errorf(codes.NotFound, "unknown datastore")
	}
	log.WithValues("datastore-name", req.GetName()[0])

	err = ds.WatchDeviations(req, stream)
	if err != nil {
		log.Error(err, "failed to watch deviations")
		return err
	}
	<-stream.Context().Done()
	log.Info("Stream context done", "errVal", stream.Context().Err())
	ds.StopDeviationsWatch(peerName)
	return nil
}

func (s *Server) datastoreToRsp(ctx context.Context, ds *datastore.Datastore) (*sdcpb.GetDataStoreResponse, error) {
	var err error
	rsp := &sdcpb.GetDataStoreResponse{
		DatastoreName: ds.Config().Name,
	}
	rsp.Target = &sdcpb.Target{
		Type:    ds.Config().SBI.Type,
		Address: ds.Config().SBI.Address,
	}
	rsp.Intents, err = ds.IntentsList(ctx)
	if err != nil {
		return nil, err
	}
	// map datastore sbi conn state to sdcpb.TargetStatus
	switch ds.ConnectionState().Status {
	case targettypes.TargetStatusConnected:
		rsp.Target.Status = sdcpb.TargetStatus_CONNECTED
	case targettypes.TargetStatusNotConnected:
		rsp.Target.Status = sdcpb.TargetStatus_NOT_CONNECTED
	default:
		rsp.Target.Status = sdcpb.TargetStatus_UNKNOWN
	}

	rsp.Schema = ds.Config().Schema.GetSchema()
	return rsp, nil
}

func (s *Server) BlameConfig(ctx context.Context, req *sdcpb.BlameConfigRequest) (*sdcpb.BlameConfigResponse, error) {
	log := logf.FromContext(ctx).WithName("BlameConfig")
	ctx = logf.IntoContext(ctx, log)

	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))

	if req.GetDatastoreName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "missing datastore name")
	}

	ds, err := s.datastores.GetDataStore(req.GetDatastoreName())
	if err != nil {
		return nil, err
	}

	tree, err := ds.BlameConfig(ctx, req.GetIncludeDefaults())
	if err != nil {
		return nil, err
	}

	return &sdcpb.BlameConfigResponse{
		ConfigTree: tree,
	}, nil

}
