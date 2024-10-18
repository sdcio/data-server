package tree

import (
	"context"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func (r *RootEntry) ToProto(ctx context.Context, onlyNewOrUpdated bool) (*sdcpb.Notification, error) {

	cacheUpdates := r.GetHighestPrecedence(onlyNewOrUpdated)

	result := &sdcpb.Notification{
		Update: make([]*sdcpb.Update, 0, len(cacheUpdates)),
	}

	// updates
	for _, cachUpdate := range cacheUpdates {
		val, err := cachUpdate.Value()
		if err != nil {
			return nil, err
		}
		path, err := cachUpdate.parentEntry.SdcpbPath()
		if err != nil {
			return nil, err
		}

		result.Update = append(result.Update, &sdcpb.Update{Path: path, Value: val})
	}

	cacheDeletes, err := r.GetDeletes(true)
	if err != nil {
		return nil, err
	}

	result.Delete = make([]*sdcpb.Path, 0, len(cacheDeletes))
	// deletes
	for _, cacheDelete := range cacheDeletes {
		path, err := cacheDelete.SdcpbPath()
		if err != nil {
			return nil, err
		}

		result.Delete = append(result.Delete, path)
	}
	return result, nil
}
