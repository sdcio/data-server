package tree

import (
	"context"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func (r *RootEntry) ToProtoUpdates(ctx context.Context, onlyNewOrUpdated bool) ([]*sdcpb.Update, error) {

	cacheUpdates := r.GetHighestPrecedence(onlyNewOrUpdated)

	upds := make([]*sdcpb.Update, 0, len(cacheUpdates))

	// updates
	for _, cachUpdate := range cacheUpdates {
		val := cachUpdate.Value()
		path, err := cachUpdate.parentEntry.SdcpbPath()
		if err != nil {
			return nil, err
		}

		upds = append(upds, &sdcpb.Update{Path: path, Value: val})
	}

	return upds, nil
}

func (r *RootEntry) ToProtoDeletes(ctx context.Context) ([]*sdcpb.Path, error) {

	cacheDeletes, err := r.GetDeletes(true)
	if err != nil {
		return nil, err
	}

	deletes := make([]*sdcpb.Path, 0, len(cacheDeletes))
	// deletes
	for _, cacheDelete := range cacheDeletes {
		path, err := cacheDelete.SdcpbPath()
		if err != nil {
			return nil, err
		}

		deletes = append(deletes, path)
	}

	return deletes, nil
}
