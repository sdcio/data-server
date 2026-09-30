package gnmi

import (
	"context"
	"errors"

	"github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/importer/proto"
	"github.com/sdcio/data-server/pkg/tree/ops"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// applyScopedRefreshFromCycleTree exports Running intent from syncTree and applies
// it scoped to configured sync paths. When export yields no Running content, Running
// is refreshed under those paths without import (nil importer).
func applyScopedRefreshFromCycleTree(ctx context.Context, runningStore types.RunningStore, syncTree *tree.RootEntry, paths []*sdcpb.Path) error {
	result, err := ops.TreeExport(syncTree.Entry, consts.RunningIntentName, consts.RunningValuesPrio, false)
	if err != nil {
		if errors.Is(err, ops.ErrorIntentNotPresent) {
			return runningStore.ApplyToRunning(ctx, paths, nil)
		}
		return err
	}
	return runningStore.ApplyToRunning(ctx, paths, proto.NewProtoTreeImporter(result))
}
