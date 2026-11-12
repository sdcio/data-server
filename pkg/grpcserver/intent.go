package grpcserver

import (
	"context"

	"github.com/sdcio/data-server/pkg/api"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) ListIntent(ctx context.Context, req *sdcpb.ListIntentRequest) (*sdcpb.ListIntentResponse, error) {
	ctx, _ = begin(ctx, "ListIntent", req, "intent-datastore", req.GetDatastoreName())
	if req.GetDatastoreName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name")
	}
	names, err := s.core.ListIntents(ctx, req.GetDatastoreName())
	if err != nil {
		return nil, notFound(err)
	}
	return &sdcpb.ListIntentResponse{DatastoreName: req.GetDatastoreName(), Intent: names}, nil
}

func (s *Server) GetIntent(ctx context.Context, req *sdcpb.GetIntentRequest) (*sdcpb.GetIntentResponse, error) {
	ctx, log := begin(ctx, "GetIntent", req, "intent-datastore", req.GetDatastoreName(), "intent-name", req.GetIntent())
	log.Info("GetIntent", "intent-format", req.GetFormat())

	if req.GetDatastoreName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name")
	}
	if req.GetIntent() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing intent name")
	}
	// the datastore lookup precedes format validation, as before
	if !s.core.DatastoreExists(req.GetDatastoreName()) {
		return nil, status.Errorf(codes.NotFound, "unknown datastore %s", req.GetDatastoreName())
	}
	format, err := intentFormat(req.GetFormat())
	if err != nil {
		return nil, err
	}
	res, err := s.core.GetIntent(ctx, api.IntentRequest{
		Datastore: req.GetDatastoreName(),
		Intent:    req.GetIntent(),
		Format:    format,
		Origin:    originFromContext(ctx),
	})
	if err != nil {
		return nil, notFound(err)
	}
	return intentResponse(req, res), nil
}
