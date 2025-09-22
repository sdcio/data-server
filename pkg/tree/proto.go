package tree

import (
	"context"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func (r *RootEntry) ToProtoUpdates(ctx context.Context, onlyNewOrUpdated bool) ([]*sdcpb.Update, error) {
	upds := r.GetHighestPrecedence(onlyNewOrUpdated)
	return upds.ToSdcpbUpdateSlice(), nil
}

func (r *RootEntry) ToProtoDeletes(ctx context.Context) ([]*sdcpb.Path, error) {
	deletes, err := r.GetDeletes(true)
	if err != nil {
		return nil, err
	}

	return deletes.SdcpbPaths(), nil
}
