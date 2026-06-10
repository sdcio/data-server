package ops

import (
	"context"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func ToProtoUpdates(ctx context.Context, e api.Entry, opts RenderOpts) ([]*sdcpb.Update, error) {
	lvs := GetHighestPrecedence(e, opts.OnlyNewOrUpdated, false, true)
	result := make([]*sdcpb.Update, 0, len(lvs))
	for _, lv := range lvs {
		if !opts.IncludeSensitive && ShouldRedact(lv.GetEntry(), false, opts.SensitivePathSet) {
			result = append(result, &sdcpb.Update{Path: lv.GetEntry().SdcpbPath(), Value: types.RedactedTypedValue})
		} else {
			result = append(result, lv.ToSdcpbUpdate())
		}
	}
	return result, nil
}

func ToProtoDeletes(ctx context.Context, e api.Entry) ([]*sdcpb.Path, error) {
	deletes, err := GetDeletes(e, true)
	if err != nil {
		return nil, err
	}

	return deletes.SdcpbPaths(), nil
}
