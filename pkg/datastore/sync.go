package datastore

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/api/adapter"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/processors"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func (d *Datastore) ApplyToRunning(ctx context.Context, deletes []*sdcpb.Path, importer importer.ImportConfigAdapter) error {

	log := logger.FromContext(ctx)

	lockStart := time.Now()
	d.syncTreeMutex.Lock()
	releaseWriteLock := sync.OnceFunc(func() {
		d.syncTreeMutex.Unlock()
		if d.syncTreeLockHoldReporter != nil {
			d.syncTreeLockHoldReporter(time.Since(lockStart))
		}
	})
	defer releaseWriteLock()

	// create a virtual task pool for delete operations
	for _, delete := range deletes {
		// navigate to delete path
		deleteRoot, err := ops.NavigateSdcpbPath(ctx, d.syncTree.Entry, delete)
		switch {
		case errors.Is(err, ops.ErrNavigateSdcpbPathNotFound):
			log.V(logger.VDebug).Info("skipping delete config subtree from internal running with no content", "path", delete.ToXPath(false))
			continue
		case err != nil:
			log.Error(err, "failed navigating to delete path", "path", delete.ToXPath(false))
			continue
		}

		deleteMarkerConfig := &processors.OwnerDeleteMarkerProcessorParams{
			Owner:        consts.RunningIntentName,
			OnlyIntended: false,
		}

		// apply delete marker, setting owner delete flag on running intent
		err = processors.NewOwnerDeleteMarker(deleteMarkerConfig).Run(deleteRoot, d.taskPool)
		if err != nil {
			log.Error(err, "failed applying delete to path", "path", delete.ToXPath(false))
			continue
		}
	}

	var importChanged bool
	// import new config if provided
	if importer != nil {
		importStats, err := d.syncTree.ImportConfig(ctx, &sdcpb.Path{}, importer, treetypes.NewUpdateInsertFlags(), d.taskPool)
		if err != nil {
			return err
		}
		importChanged = importStats.Changed()
	}

	// run remove deleted processor to clean up entries marked as deleted by owner
	rdp := processors.NewRemoveDeletedProcessor(&processors.RemoveDeletedProcessorParams{Owner: consts.RunningIntentName})
	err := rdp.Run(d.syncTree.Entry, d.taskPool)
	if err != nil {
		return err
	}

	var emptiedBranches int64
	// delete entries that have zero-length leaf variant entries after remove deleted processing
	for _, e := range rdp.GetZeroLengthLeafVariantEntries() {
		if e == nil {
			log.V(logger.VDebug).Info("skipping zero-length leaf-variant branch cleanup: nil entry in processor list")
			continue
		}
		p := e.GetParent()

		// root guard check to avoid recursing beyond the root
		if p == nil {
			log.V(logger.VDebug).Info("skipping zero-length leaf-variant branch cleanup: entry has no parent (sync tree root)",
				"path", e.SdcpbPath().ToXPath(false))
			continue
		}
		
		if err := ops.DeleteBranch(ctx, p, &sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem(e.PathName(), nil)}}, consts.RunningIntentName); err != nil {
			return err
		}
		emptiedBranches++
	}

	// conditional trace logging
	if log := log.V(logger.VTrace); log.Enabled() {
		treeExport, err := ops.TreeExport(d.syncTree.Entry, consts.RunningIntentName, consts.RunningValuesPrio, false)
		if err == nil {
			log.Info("synctree after sync apply", "content", utils.ProtoJSON(treeExport))
		}
	}

	// run reset flags processor to reset flags
	resetFlagsProcessorParams := &processors.ResetFlagsProcessorParams{DeleteFlag: true, NewFlag: true, UpdateFlag: true}
	rfp := processors.NewResetFlagsProcessor(resetFlagsProcessorParams)
	err = rfp.Run(d.syncTree.Entry, d.taskPool)
	if err != nil {
		return err
	}

	// Did the sync change running? Removed entries are gone from the tree, so each
	// kind of change is reported through a counter rather than a tree walk:
	//   - importChanged: import added or updated running leaves.
	//   - removed leaves: running leaves removed by the remove-deleted processor.
	//   - emptied branches: subtrees deleted via DeleteBranch. RemoveDeleted stops at
	//     the first deletable node and does not count the leaves below it.
	//   - reset flags: new/update/delete flags still set before reset-flags ran, which
	//     covers changes that leave only a flag (presence containers and leaf-lists
	//     inserted without ImportStats, delete flags kept where DeleteBranch is
	//     skipped at the sync-tree root). The sync tree holds only running and
	//     defaults, and defaults carry no flags, so these all belong to running.
	runningChanged := importChanged ||
		rdp.GetDeleteStatsCount() > 0 ||
		emptiedBranches > 0 ||
		rfp.GetAdjustedFlagsCount() > 0
	needDriftRevert := runningChanged || d.outstandingDriftRevert.Load()

	if !needDriftRevert {
		return nil
	}

	// Hold the write lock only for the mutate. Copy under a read lock so the
	// snapshot stays consistent without blocking other readers for the copy.
	releaseWriteLock()

	d.syncTreeMutex.RLock()
	syncTreeCopy, err := d.syncTree.DeepCopy(ctx)
	d.syncTreeMutex.RUnlock()
	if err != nil {
		d.outstandingDriftRevert.Store(true)
		return err
	}

	// TODO: this should probably be executed in a separate goroutine
	_, revertErr := d.performRevert(ctx, syncTreeCopy)
	// outstandingDriftRevert: set on any incomplete revert (prep or target apply) so the
	// next steady sync still enters drift revert when Running no longer changes; cleared only
	// after revert succeeds or we determine no target apply is needed.
	if revertErr != nil {
		d.outstandingDriftRevert.Store(true)
		return revertErr
	}
	d.outstandingDriftRevert.Store(false)
	return nil

}

func (d *Datastore) NewEmptyTree(ctx context.Context) (*tree.RootEntry, error) {
	tc := tree.NewTreeContext(d.schemaClient, d.taskPool)
	newTree, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		return nil, err
	}
	return newTree, nil
}

func (d *Datastore) performRevert(ctx context.Context, t *tree.RootEntry) (performApply bool, err error) {
	log := logger.FromContext(ctx)
	_, err = d.LoadAllButRunningIntents(ctx, t)
	if err != nil {
		return false, err
	}

	err = t.FinishInsertionPhase(ctx)
	if err != nil {
		return false, err
	}

	// TODO: optimize by checking only paths that where covered by the syncconfig
	del, err := t.GetDeletes(true)
	if err != nil {
		return false, err
	}

	performApply = len(del) > 0

	// if no deletes, check if we have updates
	if !performApply {
		updList, err := ops.ToProtoUpdates(ctx, t.Entry, true)
		if err != nil {
			return false, err
		}
		performApply = len(updList) > 0
	}

	if !performApply {
		return false, nil
	}

	log.Info("reverting after sync")
	resp, applyErr := d.applyIntent(ctx, adapter.NewEntryOutputAdapter(t.Entry))
	if applyErr != nil {
		log.Error(applyErr, "failed applying deviations to running", "response", utils.ProtoJSON(resp))
		return true, applyErr
	}
	return true, nil
}
