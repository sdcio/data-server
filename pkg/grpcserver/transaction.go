package grpcserver

import (
	"context"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/utils"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func (s *Server) TransactionSet(ctx context.Context, req *sdcpb.TransactionSetRequest) (*sdcpb.TransactionSetResponse, error) {
	log := logf.FromContext(ctx).WithName("TransactionSet").WithName("transaction").WithValues(
		"transaction-id", req.GetTransactionId(),
		"datastore-name", req.GetDatastoreName(),
	)
	ctx = logf.IntoContext(ctx, log)
	log.Info("TransactionSet",
		"transaction-dry-run", req.GetDryRun(),
		"transaction-timeout", req.GetTimeout(),
		"transaction-peer", originFromContext(ctx),
	)
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))

	if req.GetDatastoreName() == "" {
		return nil, transactionMissingName()
	}

	intents := make([]api.Intent, 0, len(req.GetIntents()))
	for _, pb := range req.GetIntents() {
		in, err := intentFromSdcpb(pb)
		if err != nil {
			return nil, err
		}
		intents = append(intents, in)
	}

	opts := setOptionsFromRequest(req, originFromContext(ctx))
	res, err := s.core.TransactionSet(ctx, api.TxRef{Datastore: req.GetDatastoreName(), ID: req.GetTransactionId()}, opts, intents)
	if err != nil {
		if mapped := statusNotFound(err); mapped != err {
			return nil, mapped
		}
		return nil, translateLockedOrNotConnected(err)
	}
	return transactionResultToSdcpb(res), nil
}

func (s *Server) TransactionConfirm(ctx context.Context, req *sdcpb.TransactionConfirmRequest) (*sdcpb.TransactionConfirmResponse, error) {
	log := logf.FromContext(ctx).WithName("TransactionConfirm").WithName("transaction").WithValues(
		"transaction-id", req.GetTransactionId(),
		"datastore-name", req.GetDatastoreName(),
	)
	ctx = logf.IntoContext(ctx, log)
	log.Info("TransactionConfirm", "transaction-peer", originFromContext(ctx))
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))

	if req.GetDatastoreName() == "" {
		return nil, transactionMissingName()
	}
	err := s.core.TransactionConfirm(ctx, api.TxRef{Datastore: req.GetDatastoreName(), ID: req.GetTransactionId()}, originFromContext(ctx))
	if err != nil {
		if mapped := statusNotFound(err); mapped != err {
			return nil, mapped
		}
		return nil, translateLockedOrNotConnected(err)
	}
	return &sdcpb.TransactionConfirmResponse{}, nil
}

func (s *Server) TransactionCancel(ctx context.Context, req *sdcpb.TransactionCancelRequest) (*sdcpb.TransactionCancelResponse, error) {
	log := logf.FromContext(ctx).WithName("TransactionCancel").WithName("transaction").WithValues(
		"transaction-id", req.GetTransactionId(),
		"datastore-name", req.GetDatastoreName(),
	)
	ctx = logf.IntoContext(ctx, log)
	log.Info("TransactionCancel", "transaction-peer", originFromContext(ctx))
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))

	if req.GetDatastoreName() == "" {
		return nil, transactionMissingName()
	}
	err := s.core.TransactionCancel(ctx, api.TxRef{Datastore: req.GetDatastoreName(), ID: req.GetTransactionId()}, originFromContext(ctx))
	if err != nil {
		if mapped := statusNotFound(err); mapped != err {
			return nil, mapped
		}
		return nil, translateLockedOrNotConnected(err)
	}
	return &sdcpb.TransactionCancelResponse{}, nil
}
