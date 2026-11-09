package grpcserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/utils"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) ListDataStore(ctx context.Context, req *sdcpb.ListDataStoreRequest) (*sdcpb.ListDataStoreResponse, error) {
	log := logf.FromContext(ctx).WithName("ListDataStore")
	ctx = logf.IntoContext(ctx, log)
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))

	infos, err := s.core.ListDataStores(ctx)
	if err != nil {
		return nil, err
	}
	rs := make([]*sdcpb.GetDataStoreResponse, 0, len(infos))
	for _, info := range infos {
		r, err := datastoreInfoToSdcpb(info)
		if err != nil {
			return nil, err
		}
		rs = append(rs, r)
	}
	return &sdcpb.ListDataStoreResponse{Datastores: rs}, nil
}

func (s *Server) GetDataStore(ctx context.Context, req *sdcpb.GetDataStoreRequest) (*sdcpb.GetDataStoreResponse, error) {
	log := logf.FromContext(ctx).WithName("GetDataStore").WithValues("datastore-name", req.GetDatastoreName())
	ctx = logf.IntoContext(ctx, log)
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))

	if req.GetDatastoreName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name attribute")
	}
	info, err := s.core.GetDataStore(ctx, req.GetDatastoreName())
	if err != nil {
		return nil, err
	}
	return datastoreInfoToSdcpb(info)
}

func (s *Server) CreateDataStore(ctx context.Context, req *sdcpb.CreateDataStoreRequest) (*sdcpb.CreateDataStoreResponse, error) {
	log := logf.FromContext(ctx).WithName("CreateDataStore").WithValues("datastore-name", req.GetDatastoreName())
	log.Info("creating datastore",
		"datastore-schema", utils.ProtoJSON(req.GetSchema()),
		"datastore-target", utils.ProtoJSON(req.GetTarget()),
	)
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))

	spec, err := datastoreSpecFromCreate(req)
	if err != nil {
		if errors.Is(err, api.ErrInvalidArgument) {
			if spec.Name == "" && len(req.GetDatastoreName()) == 0 {
				log.Error(nil, "missing datastore name attribute")
				return nil, status.Error(codes.InvalidArgument, "missing datastore name attribute")
			}
			return nil, statusInvalidArgument(err)
		}
		return nil, err
	}

	if _, err := s.core.GetDataStore(ctx, spec.Name); err == nil {
		log.Error(nil, "datastore already exists")
		return nil, status.Errorf(codes.InvalidArgument, "datastore %s already exists", spec.Name)
	}

	if err := s.core.CreateDataStore(ctx, spec); err != nil {
		if errors.Is(err, api.ErrInvalidArgument) {
			log.Error(err, "invalid datastore config")
			return nil, status.Errorf(codes.InvalidArgument, "invalid datastore config: %v", err)
		}
		log.Error(err, "failed creating new datastore")
		return nil, err
	}
	log.Info("datastore created successfully")
	return &sdcpb.CreateDataStoreResponse{}, nil
}

func (s *Server) DeleteDataStore(ctx context.Context, req *sdcpb.DeleteDataStoreRequest) (*sdcpb.DeleteDataStoreResponse, error) {
	log := logf.FromContext(ctx).WithName("DeleteDataStore")
	ctx = logf.IntoContext(ctx, log)
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))

	name := req.GetName()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name attribute")
	}
	if err := s.core.DeleteDataStore(ctx, name); err != nil {
		if errors.Is(err, api.ErrUnknownDatastore) {
			return nil, err
		}
		if errors.Is(err, api.ErrInvalidArgument) {
			return nil, statusInvalidArgument(err)
		}
		log.Error(err, "failed to delete datastore")
		return nil, fmt.Errorf("failed to delete datastore: %w", err)
	}
	log.Info("deleted datastore")
	return &sdcpb.DeleteDataStoreResponse{}, nil
}

func (s *Server) BlameConfig(ctx context.Context, req *sdcpb.BlameConfigRequest) (*sdcpb.BlameConfigResponse, error) {
	log := logf.FromContext(ctx).WithName("BlameConfig")
	ctx = logf.IntoContext(ctx, log)
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))

	if req.GetDatastoreName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "missing datastore name")
	}
	tree, err := s.core.BlameConfig(ctx, req.GetDatastoreName(), req.GetIncludeDefaults())
	if err != nil {
		return nil, err
	}
	return &sdcpb.BlameConfigResponse{ConfigTree: blameNodeToSdcpb(tree)}, nil
}
