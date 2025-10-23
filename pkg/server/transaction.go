package server

import (
	"context"
	"errors"
	"time"

	"github.com/sdcio/data-server/pkg/datastore"
	"github.com/sdcio/data-server/pkg/datastore/types"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/utils"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func (s *Server) TransactionSet(ctx context.Context, req *sdcpb.TransactionSetRequest) (*sdcpb.TransactionSetResponse, error) {
	pr, _ := peer.FromContext(ctx)

	log := logf.FromContext(ctx).WithName("TransactionSet")
	log = log.WithName("transaction").WithValues(
		"transaction-id", req.GetTransactionId(),
	)
	ctx = logf.IntoContext(ctx, log)

	log.Info("TransactionSet",
		"transaction-datastore-name", req.GetDatastoreName(),
		"transaction-dry-run", req.GetDryRun(),
		"transaction-timeout", req.GetTimeout(),
		"transaction-peer", pr.String(),
	)

	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))

	if req.GetDatastoreName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name")
	}

	// load either default timer from global config or use the request provided value as seconds
	var timeout time.Duration
	if req.Timeout == nil {
		timeout = s.config.DefaultTransactionTimeout
	} else {
		timeout = time.Duration(*req.Timeout) * time.Second
	}

	// retrieve the referenced datastore
	ds, err := s.datastores.getDataStore(req.DatastoreName)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// create a list of intents that are contained in the req.
	transactionIntents := make([]*types.TransactionIntent, 0, len(req.GetIntents()))

	// populate the transactionIntents from the req.intents basically transforming from sdcpb to datastore intent format
	for _, intent := range req.GetIntents() {
		ti, err := ds.SdcpbTransactionIntentToInternalTI(ctx, intent)
		if err != nil {
			return nil, err
		}

		transactionIntents = append(transactionIntents, ti)
	}

	// perform the conversion also for the replace intent
	var replaceIntent *types.TransactionIntent
	if req.GetReplaceIntent() != nil {
		// overwrite replace priority and name with specific value
		req.ReplaceIntent.Priority = tree.ReplaceValuesPrio
		req.ReplaceIntent.Intent = tree.ReplaceIntentName

		replaceIntent, err = ds.SdcpbTransactionIntentToInternalTI(ctx, req.GetReplaceIntent())
		if err != nil {
			return nil, err
		}
	}

	// with the transformed intents call the TransactionSet() on the datastore.
	rsp, err := ds.TransactionSet(ctx, req.GetTransactionId(), transactionIntents, replaceIntent, timeout, req.GetDryRun())
	return rsp, translateInternalToGrpcError(err)
}

func (s *Server) TransactionConfirm(ctx context.Context, req *sdcpb.TransactionConfirmRequest) (*sdcpb.TransactionConfirmResponse, error) {
	pr, _ := peer.FromContext(ctx)

	log := logf.FromContext(ctx).WithName("TransactionConfirm")
	log = log.WithName("transaction").WithValues(
		"transaction-id", req.GetTransactionId(),
	)
	ctx = logf.IntoContext(ctx, log)

	log.Info("TransactionConfirm",
		"transaction-datastore-name", req.GetDatastoreName(),
		"transaction-peer", pr.String(),
	)

	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))

	if req.GetDatastoreName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name")
	}

	ds, err := s.datastores.GetDataStore(req.DatastoreName)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	err = ds.TransactionConfirm(ctx, req.TransactionId)
	return nil, translateInternalToGrpcError(err)
}

func (s *Server) TransactionCancel(ctx context.Context, req *sdcpb.TransactionCancelRequest) (*sdcpb.TransactionCancelResponse, error) {
	pr, _ := peer.FromContext(ctx)

	log := logf.FromContext(ctx).WithName("TransactionCancel")
	log = log.WithName("transaction").WithValues(
		"transaction-id", req.GetTransactionId(),
	)
	ctx = logf.IntoContext(ctx, log)

	log.Info("TransactionCancel",
		"transaction-datastore-name", req.GetDatastoreName(),
		"transaction-peer", pr.String(),
	)

	log.V(logf.VDebug).Info("received request", "raw-request", utils.FormatProtoJSON(req))

	if req.GetDatastoreName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name")
	}

	ds, err := s.datastores.GetDataStore(req.DatastoreName)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	err = ds.TransactionCancel(ctx, req.TransactionId)
	return nil, translateInternalToGrpcError(err)
}

// translateInternalToGrpcError central function to map internal errors to grpc error codes
func translateInternalToGrpcError(err error) error {
	if errors.Is(err, datastore.ErrDatastoreLocked) {
		return status.Error(codes.Aborted, err.Error())
	}
	return err
}
