package ops

import (
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/types"
)

// RunningSyncChangeInput summarizes structural Running changes applied during sync
// (before reset-flags), excluding revert intent loading.
type RunningSyncChangeInput struct {
	// ImportChanged is true when the import stage added or updated Running leaves.
	ImportChanged      bool
	RemovedLeafCount   int64
	EmptiedBranchCount int64
}

// RunningChangedDuringSync reports whether the sync changed Running: new, updated, or
// removed leaves, branches emptied by removal, or Running leaf variants still marked
// with new/update/delete flags prior to reset-flags.
func RunningChangedDuringSync(root api.Entry, in RunningSyncChangeInput) bool {
	if in.ImportChanged || in.RemovedLeafCount > 0 || in.EmptiedBranchCount > 0 {
		return true
	}
	return runningHasPendingRunningFlags(root)
}

func runningHasPendingRunningFlags(e api.Entry) bool {
	if HoldsLeafVariants(e) {
		if le := e.GetLeafVariants().GetByOwner(consts.RunningIntentName); le != nil {
			if le.GetNewFlag() || le.GetUpdateFlag() || le.GetDeleteFlag() {
				return true
			}
		}
	}
	for _, c := range e.GetChilds(types.DescendMethodAll) {
		if runningHasPendingRunningFlags(c) {
			return true
		}
	}
	return false
}
