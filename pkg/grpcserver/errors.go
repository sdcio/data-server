package grpcserver

import (
	"errors"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/datastore"
	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// translateLockedOrNotConnected maps datastore and target sentinels to gRPC
// codes, matching pkg/server.translateInternalToGrpcError.
func translateLockedOrNotConnected(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, api.ErrDatastoreLocked), errors.Is(err, datastore.ErrDatastoreLocked):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, api.ErrNotConnected), errors.Is(err, targettypes.ErrNotConnected):
		return status.Error(codes.Unavailable, err.Error())
	default:
		return err
	}
}

func statusInvalidArgument(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, api.ErrInvalidArgument) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return err
}

func statusNotFound(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, api.ErrUnknownDatastore) {
		return status.Error(codes.NotFound, err.Error())
	}
	return err
}
