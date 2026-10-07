package ops

import (
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// CollectRunningSyncRevertScopes returns the revert scopes (see "Revert scope" in CONTEXT.md) of all
// entries whose Running variant was touched by a sync: new, updated, or marked deleted.
//
// It has to run before the remove deleted and reset flags processors, which destroy the flags
// and remove the deleted entries.
func CollectRunningSyncRevertScopes(root api.Entry) *sdcpb.PathSet {
	set := sdcpb.NewPathSet()
	collectRunningSyncRevertScopes(root, set)
	return set
}

func collectRunningSyncRevertScopes(e api.Entry, set *sdcpb.PathSet) {
	if HoldsLeafVariants(e) {
		if le := e.GetLeafVariants().GetByOwner(consts.RunningIntentName); le != nil {
			if le.GetNewFlag() || le.GetUpdateFlag() || le.GetDeleteFlag() {
				set.AddPath(RevertScope(e))
			}
		}
	}
	for _, c := range e.GetChilds(types.DescendMethodAll) {
		collectRunningSyncRevertScopes(c, set)
	}
}
