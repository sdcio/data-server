package server

import (
	"context"

	"github.com/sdcio/data-server/pkg/datastore"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func (s *Server) TransactionSet(ctx context.Context, req *sdcpb.TransactionSetRequest) (*sdcpb.TransactionSetResponse, error) {
	pr, _ := peer.FromContext(ctx)
	log.Debugf("received TransactionSetRequest request %v from peer %s", req, pr.Addr.String())

	if req.GetDatastoreName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name")
	}

	ds, err := s.getDataStore(req.DatastoreName)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	setTransaction := datastore.NewTransaction(req.TransactionId)

	for _, intent := range req.GetIntents() {
		ti, err := ds.SdcpbTransactionIntentToInternalTI(ctx, intent)
		if err != nil {
			return nil, err
		}
		setTransaction.AddTransactionIntent(ti)
	}

	return ds.TransactionSet(ctx, req.GetTransactionId(), req.GetDryRun(), req.GetIntents())
}

func (s *Server) TransactionConfirm(ctx context.Context, req *sdcpb.TransactionConfirmRequest) (*sdcpb.TransactionConfirmResponse, error) {
	pr, _ := peer.FromContext(ctx)
	log.Debugf("received TransactionConfirm request %v from peer %s", req, pr.Addr.String())

	if req.GetDatastoreName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name")
	}

	ds, err := s.getDataStore(req.DatastoreName)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return nil, ds.TransactionConfirm(ctx, req.TransactionId)
}

func (s *Server) TransactionCancel(ctx context.Context, req *sdcpb.TransactionCancelRequest) (*sdcpb.TransactionCancelResponse, error) {
	pr, _ := peer.FromContext(ctx)
	log.Debugf("received TransactionCancel request %v from peer %s", req, pr.Addr.String())

	if req.GetDatastoreName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name")
	}

	ds, err := s.getDataStore(req.DatastoreName)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return nil, ds.TransactionCancel(ctx, req.TransactionId)
}
