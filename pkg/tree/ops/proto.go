package ops

import (
	"context"

	"github.com/sdcio/data-server/pkg/tree/api"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func ToProtoUpdates(ctx context.Context, e api.Entry, onlyNewOrUpdated bool) ([]*sdcpb.Update, error) {
	result := api.LeafVariantSlice{}
	result = GetHighestPrecedence(e, onlyNewOrUpdated, false, true)
	return result.ToSdcpbUpdateSlice(), nil
}

func ToProtoDeletes(ctx context.Context, e api.Entry) ([]*sdcpb.Path, error) {
	deletes, err := GetDeletes(e, true)
	if err != nil {
		return nil, err
	}

	return deletes.SdcpbPaths(), nil
}
