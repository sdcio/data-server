package grpcserver

import (
	"context"
	"errors"
	"strings"

	"github.com/sdcio/data-server/pkg/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// notFound maps api.ErrUnknownDatastore to NotFound and passes every other
// error through unchanged.
func notFound(err error) error {
	if errors.Is(err, api.ErrUnknownDatastore) {
		return status.Error(codes.NotFound, err.Error())
	}
	return err
}

// invalidArgument returns an InvalidArgument status for api.ErrInvalidArgument
// errors, with the sentinel text removed from the message. Other errors pass
// through unchanged.
func invalidArgument(err error) error {
	if errors.Is(err, api.ErrInvalidArgument) {
		return status.Error(codes.InvalidArgument, trimSentinel(err))
	}
	return err
}

// transactionError maps core errors of the transaction RPCs to the codes the
// controller relies on: NotFound, Aborted (locked), Unavailable (not
// connected), ResourceExhausted and DeadlineExceeded. Invalid arguments stay
// untyped, as they were before the core existed. Anything else passes through.
func transactionError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, api.ErrUnknownDatastore):
		return notFound(err)
	case errors.Is(err, api.ErrDatastoreLocked):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, api.ErrNotConnected):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, api.ErrResourceExhausted):
		return status.Error(codes.ResourceExhausted, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	case errors.Is(err, api.ErrInvalidArgument):
		return errors.New(trimSentinel(err))
	default:
		return err
	}
}

func trimSentinel(err error) string {
	return strings.TrimPrefix(err.Error(), api.ErrInvalidArgument.Error()+": ")
}
