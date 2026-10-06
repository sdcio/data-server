package ops

import (
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// CollectRunningSyncChangedPaths returns paths whose Running variant was touched
// during sync (new, updated, or delete flags) before reset-flags runs.
func CollectRunningSyncChangedPaths(root api.Entry) []*sdcpb.Path {
	set := sdcpb.NewPathSet()
	collectRunningSyncChangedPaths(root, set)
	return set.ToPathSlice()
}

func collectRunningSyncChangedPaths(e api.Entry, set *sdcpb.PathSet) {
	if HoldsLeafVariants(e) {
		if le := e.GetLeafVariants().GetByOwner(consts.RunningIntentName); le != nil {
			if le.GetNewFlag() || le.GetUpdateFlag() || le.GetDeleteFlag() {
				set.AddPaths([]*sdcpb.Path{e.SdcpbPath()})
			}
		}
	}
	for _, c := range e.GetChilds(types.DescendMethodAll) {
		collectRunningSyncChangedPaths(c, set)
	}
}

// RevertScopesFromChangedPaths maps changed entry paths to the subtree roots that
// must be present for precedence / choice evaluation (typically the list entry).
func RevertScopesFromChangedPaths(changed []*sdcpb.Path) []*sdcpb.Path {
	set := sdcpb.NewPathSet()
	for _, p := range changed {
		if p == nil {
			continue
		}
		set.AddPaths([]*sdcpb.Path{revertScopePath(p)})
	}
	return set.ToPathSlice()
}

func revertScopePath(changed *sdcpb.Path) *sdcpb.Path {
	elems := changed.GetElem()
	if len(elems) == 0 {
		return changed
	}
	// Leaf paths end with a schema leaf; use the parent list/container path as scope.
	if len(elems) >= 2 {
		return &sdcpb.Path{Elem: append([]*sdcpb.PathElem(nil), elems[:len(elems)-1]...)}
	}
	return changed
}
