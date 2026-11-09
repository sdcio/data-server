package grpcserver

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/sdcio/data-server/pkg/api"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type txRequest interface {
	proto.Message
	GetTransactionId() string
	GetDatastoreName() string
}

// beginTx starts a transaction RPC: it installs the transaction logger and
// returns the transaction reference, failing with InvalidArgument when the
// datastore name is missing.
func beginTx(ctx context.Context, op string, req txRequest) (context.Context, logr.Logger, api.TxRef, error) {
	log := logf.FromContext(ctx).WithName(op).WithName("transaction").WithValues(
		"transaction-id", req.GetTransactionId(),
		"datastore-name", req.GetDatastoreName(),
	)
	ctx, log = beginWith(ctx, log, req)
	log.Info(op, "transaction-peer", originFromContext(ctx))
	if req.GetDatastoreName() == "" {
		return ctx, log, api.TxRef{}, status.Error(codes.InvalidArgument, "missing datastore name")
	}
	return ctx, log, api.TxRef{Datastore: req.GetDatastoreName(), ID: req.GetTransactionId()}, nil
}

func (s *Server) TransactionSet(ctx context.Context, req *sdcpb.TransactionSetRequest) (*sdcpb.TransactionSetResponse, error) {
	ctx, log, ref, err := beginTx(ctx, "TransactionSet", req)
	if err != nil {
		return nil, err
	}
	log.Info("transaction options", "transaction-dry-run", req.GetDryRun(), "transaction-timeout", req.GetTimeout())

	intents := make([]api.Intent, 0, len(req.GetIntents()))
	for _, pb := range req.GetIntents() {
		in, err := intent(pb)
		if err != nil {
			return nil, err
		}
		intents = append(intents, in)
	}
	opts, err := setOptions(req, originFromContext(ctx))
	if err != nil {
		return nil, err
	}

	res, err := s.core.TransactionSet(ctx, ref, opts, intents)
	if err != nil {
		return nil, transactionError(err)
	}
	return transactionResponse(res), nil
}

func (s *Server) TransactionConfirm(ctx context.Context, req *sdcpb.TransactionConfirmRequest) (*sdcpb.TransactionConfirmResponse, error) {
	ctx, _, ref, err := beginTx(ctx, "TransactionConfirm", req)
	if err != nil {
		return nil, err
	}
	if err := s.core.TransactionConfirm(ctx, ref, originFromContext(ctx)); err != nil {
		return nil, transactionError(err)
	}
	return &sdcpb.TransactionConfirmResponse{}, nil
}

func (s *Server) TransactionCancel(ctx context.Context, req *sdcpb.TransactionCancelRequest) (*sdcpb.TransactionCancelResponse, error) {
	ctx, _, ref, err := beginTx(ctx, "TransactionCancel", req)
	if err != nil {
		return nil, err
	}
	if err := s.core.TransactionCancel(ctx, ref, originFromContext(ctx)); err != nil {
		return nil, transactionError(err)
	}
	return &sdcpb.TransactionCancelResponse{}, nil
}
