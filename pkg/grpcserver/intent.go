package grpcserver

import (
	"context"
	"errors"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/utils"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) ListIntent(ctx context.Context, req *sdcpb.ListIntentRequest) (*sdcpb.ListIntentResponse, error) {
	log := logf.FromContext(ctx).WithName("ListIntent").WithValues("intent-datastore", req.GetDatastoreName())
	ctx = logf.IntoContext(ctx, log)
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))

	if req.GetDatastoreName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name")
	}
	names, err := s.core.ListIntents(ctx, req.GetDatastoreName())
	if err != nil {
		if mapped := statusNotFound(err); mapped != err {
			return nil, mapped
		}
		return nil, err
	}
	return &sdcpb.ListIntentResponse{DatastoreName: req.GetDatastoreName(), Intent: names}, nil
}

func (s *Server) GetIntent(ctx context.Context, req *sdcpb.GetIntentRequest) (*sdcpb.GetIntentResponse, error) {
	log := logf.FromContext(ctx).WithName("GetIntent").WithValues(
		"intent-datastore", req.GetDatastoreName(),
		"intent-name", req.GetIntent(),
	)
	ctx = logf.IntoContext(ctx, log)
	log.Info("GetIntent", "intent-format", req.GetFormat())
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))

	if req.GetDatastoreName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name")
	}
	if req.GetIntent() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing intent name")
	}
	format, err := intentFormatFromSdcpb(req.GetFormat())
	if err != nil {
		return nil, statusInvalidArgument(err)
	}
	res, err := s.core.GetIntent(ctx, api.IntentRequest{
		Datastore: req.GetDatastoreName(),
		Intent:    req.GetIntent(),
		Format:    format,
		Origin:    originFromContext(ctx),
	})
	if err != nil {
		if mapped := statusNotFound(err); mapped != err {
			return nil, mapped
		}
		if errors.Is(err, api.ErrInvalidArgument) {
			return nil, statusInvalidArgument(err)
		}
		return nil, err
	}
	return intentResultToSdcpb(req, res)
}
