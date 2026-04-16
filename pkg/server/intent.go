package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sdcio/data-server/pkg/utils"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) ListIntent(ctx context.Context, req *sdcpb.ListIntentRequest) (*sdcpb.ListIntentResponse, error) {
	log := logf.FromContext(ctx).WithName("ListIntent")
	log = log.WithValues(
		"intent-datastore", req.GetDatastoreName(),
	)
	ctx = logf.IntoContext(ctx, log)

	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))

	if req.GetDatastoreName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name")
	}

	ds, err := s.datastores.getDataStore(req.GetDatastoreName())
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	resp := &sdcpb.ListIntentResponse{
		DatastoreName: req.DatastoreName,
	}
	intentNames, err := ds.IntentsList(ctx)
	if err != nil {
		return nil, err
	}
	resp.Intent = intentNames
	return resp, nil
}

func (s *Server) GetIntent(ctx context.Context, req *sdcpb.GetIntentRequest) (*sdcpb.GetIntentResponse, error) {
	log := logf.FromContext(ctx).WithName("GetIntent")
	log = log.WithValues(
		"intent-datastore", req.GetDatastoreName(),
		"intent-name", req.GetIntent(),
	)
	ctx = logf.IntoContext(ctx, log)
	log.Info("GetIntent",
		"intent-format", req.GetFormat(),
	)
	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))

	if req.GetDatastoreName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name")
	}

	if req.GetIntent() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing intent name")
	}
	// retrieve the referenced datastore
	ds, err := s.datastores.getDataStore(req.GetDatastoreName())
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	rsp, err := ds.GetIntent(ctx, req.GetIntent())
	if err != nil {
		return nil, err
	}

	getIntentResponse := &sdcpb.GetIntentResponse{
		DatastoreName: req.GetDatastoreName(),
		Format:        req.GetFormat(),
		IntentName:    req.GetIntent(),
		Orphan:        rsp.IsOrphan(),
		Priority:      rsp.GetPriority(),
		NonRevertive:  rsp.IsNonRevertive(),
		Deletes:       rsp.GetExplicitDeletes(),
	}

	switch req.GetFormat() {
	case sdcpb.Format_Intent_Format_JSON, sdcpb.Format_Intent_Format_JSON_IETF:
		var j any
		switch req.GetFormat() {
		case sdcpb.Format_Intent_Format_JSON:
			j, err = rsp.ToJson(ctx)
			if err != nil {
				return nil, err
			}
		case sdcpb.Format_Intent_Format_JSON_IETF:
			j, err = rsp.ToJsonIETF(ctx)
			if err != nil {
				return nil, err
			}
		}
		b, err := json.Marshal(j)
		if err != nil {
			return nil, err
		}
		getIntentResponse.Intent = &sdcpb.GetIntentResponse_Blob{
			Blob: b,
		}
		return getIntentResponse, nil

	case sdcpb.Format_Intent_Format_XML:
		doc, err := rsp.ToXML(ctx)
		if err != nil {
			return nil, err
		}
		xml, err := doc.WriteToBytes()
		if err != nil {
			return nil, err
		}
		getIntentResponse.Intent = &sdcpb.GetIntentResponse_Blob{
			Blob: xml,
		}
		return getIntentResponse, nil
	case sdcpb.Format_Intent_Format_PROTO:
		upds, err := rsp.ToProtoUpdates(ctx)
		if err != nil {
			return nil, err
		}
		getIntentResponse.Intent = &sdcpb.GetIntentResponse_Proto{
			Proto: &sdcpb.Intent{
				Intent:   rsp.GetIntentName(),
				Priority: rsp.GetPriority(),
				Update:   upds,
			},
		}
		return getIntentResponse, nil
	}
	return nil, fmt.Errorf("unknown format")
}
