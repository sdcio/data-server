package grpcserver

import (
	"context"
	"fmt"
	"math"

	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) ListDataStore(ctx context.Context, req *sdcpb.ListDataStoreRequest) (*sdcpb.ListDataStoreResponse, error) {
	ctx, _ = begin(ctx, "ListDataStore", req)
	infos, err := s.core.ListDataStores(ctx)
	if err != nil {
		return nil, err
	}
	rs := make([]*sdcpb.GetDataStoreResponse, 0, len(infos))
	for _, info := range infos {
		rs = append(rs, datastoreResponse(info))
	}
	return &sdcpb.ListDataStoreResponse{Datastores: rs}, nil
}

func (s *Server) GetDataStore(ctx context.Context, req *sdcpb.GetDataStoreRequest) (*sdcpb.GetDataStoreResponse, error) {
	ctx, _ = begin(ctx, "GetDataStore", req, "datastore-name", req.GetDatastoreName())
	if req.GetDatastoreName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name attribute")
	}
	info, err := s.core.GetDataStore(ctx, req.GetDatastoreName())
	if err != nil {
		return nil, err
	}
	return datastoreResponse(info), nil
}

// CreateDataStore validates the name, rejects an existing datastore, then
// converts and validates target and sync, in that order, before creating it.
func (s *Server) CreateDataStore(ctx context.Context, req *sdcpb.CreateDataStoreRequest) (*sdcpb.CreateDataStoreResponse, error) {
	ctx, log := begin(ctx, "CreateDataStore", req, "datastore-name", req.GetDatastoreName())
	log.Info("creating datastore",
		"datastore-schema", utils.ProtoJSON(req.GetSchema()),
		"datastore-target", utils.ProtoJSON(req.GetTarget()),
	)

	name := req.GetDatastoreName()
	switch {
	case len(name) == 0:
		log.Error(nil, "missing datastore name attribute")
		return nil, status.Error(codes.InvalidArgument, "missing datastore name attribute")
	case len(name) > math.MaxUint16:
		log.Error(nil, "datastore name attribute too long")
		return nil, status.Error(codes.InvalidArgument, "datastore name attribute too long")
	case s.core.DatastoreExists(name):
		log.Error(nil, "datastore already exists")
		return nil, status.Errorf(codes.InvalidArgument, "datastore %s already exists", name)
	}

	spec, err := datastoreSpec(req)
	if err != nil {
		log.Error(err, "invalid datastore request")
		return nil, err
	}
	if err := s.core.CreateDataStore(ctx, spec); err != nil {
		log.Error(err, "failed creating new datastore")
		return nil, invalidArgument(err)
	}
	log.Info("datastore created successfully")
	return &sdcpb.CreateDataStoreResponse{}, nil
}

func (s *Server) DeleteDataStore(ctx context.Context, req *sdcpb.DeleteDataStoreRequest) (*sdcpb.DeleteDataStoreResponse, error) {
	ctx, log := begin(ctx, "DeleteDataStore", req)
	name := req.GetName()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name attribute")
	}
	if !s.core.DatastoreExists(name) {
		return nil, fmt.Errorf("unknown datastore %s", name)
	}
	if err := s.core.DeleteDataStore(ctx, name); err != nil {
		log.Error(err, "failed to delete datastore")
		return nil, fmt.Errorf("failed to delete datastore: %w", err)
	}
	log.Info("deleted datastore")
	return &sdcpb.DeleteDataStoreResponse{}, nil
}

func (s *Server) BlameConfig(ctx context.Context, req *sdcpb.BlameConfigRequest) (*sdcpb.BlameConfigResponse, error) {
	ctx, _ = begin(ctx, "BlameConfig", req)
	if req.GetDatastoreName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "missing datastore name")
	}
	tree, err := s.core.BlameConfig(ctx, req.GetDatastoreName(), req.GetIncludeDefaults())
	if err != nil {
		return nil, err
	}
	return &sdcpb.BlameConfigResponse{ConfigTree: blameTree(tree)}, nil
}
