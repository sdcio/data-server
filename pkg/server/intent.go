package server

import (
	"context"
	"encoding/json"
	"fmt"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) ListIntent(ctx context.Context, req *sdcpb.ListIntentRequest) (*sdcpb.ListIntentResponse, error) {
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

	switch req.GetFormat() {
	case sdcpb.Format_Intent_Format_JSON, sdcpb.Format_Intent_Format_JSON_IETF:
		var j any
		switch req.GetFormat() {
		case sdcpb.Format_Intent_Format_JSON:
			j, err = rsp.ToJson()
			if err != nil {
				return nil, err
			}
		case sdcpb.Format_Intent_Format_JSON_IETF:
			j, err = rsp.ToJsonIETF()
			if err != nil {
				return nil, err
			}
		}
		b, err := json.Marshal(j)
		if err != nil {
			return nil, err
		}
		return &sdcpb.GetIntentResponse{
			DatastoreName: req.GetIntent(),
			Format:        req.GetFormat(),
			Intent: &sdcpb.GetIntentResponse_Blob{
				Blob: b,
			},
		}, nil

	case sdcpb.Format_Intent_Format_XML:
		doc, err := rsp.ToXML()
		if err != nil {
			return nil, err
		}
		xml, err := doc.WriteToBytes()
		if err != nil {
			return nil, err
		}

		return &sdcpb.GetIntentResponse{
			DatastoreName: req.GetIntent(),
			Format:        req.GetFormat(),
			Intent: &sdcpb.GetIntentResponse_Blob{
				Blob: xml,
			},
		}, nil
	case sdcpb.Format_Intent_Format_PROTO:
		upds, err := rsp.ToProtoUpdates(ctx)
		if err != nil {
			return nil, err
		}

		return &sdcpb.GetIntentResponse{
			DatastoreName: req.GetIntent(),
			Format:        req.GetFormat(),
			Intent: &sdcpb.GetIntentResponse_Proto{
				Proto: &sdcpb.Intent{
					Intent: req.GetIntent(),
					Update: upds,
				},
			},
		}, nil
	}
	return nil, fmt.Errorf("unknown format")
}
