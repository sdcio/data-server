package datastore

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/api"
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

// ApplyToRunning commits one sync observation. syncStartedAt is Unix
// nanoseconds captured before the producer starts reading the target snapshot.
func (d *Datastore) ApplyToRunning(ctx context.Context, syncStartedAt int64, deletes []*sdcpb.Path, importer importer.ImportConfigAdapter) error {

	log := logger.FromContext(ctx)

	d.syncTreeMutex.Lock()
	syncTreeUnlock := sync.OnceFunc(d.syncTreeMutex.Unlock)

	defer syncTreeUnlock()

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

	// import new config if provided
	var importedPaths map[canonicalLeafPath]bool
	if importer != nil {
		probes := d.beginPendingImportProbes(ctx, syncStartedAt, deletes)
		_, importErr := d.syncTree.ImportConfig(ctx, &sdcpb.Path{}, importer, treetypes.NewUpdateInsertFlags(), d.taskPool)
		importedPaths = d.finishPendingImportProbes(ctx, probes)
		if importErr != nil {
			return importErr
		}
	}

	// run remove deleted processor to clean up entries marked as deleted by owner
	rdp := processors.NewRemoveDeletedProcessor(&processors.RemoveDeletedProcessorParams{Owner: consts.RunningIntentName})
	err := rdp.Run(d.syncTree.Entry, d.taskPool)
	if err != nil {
		return err
	}

	// delete entries that have zero-length leaf variant entries after remove deleted processing
	for _, e := range rdp.GetZeroLengthLeafVariantEntries() {
		if e == nil {
			log.V(logger.VDebug).Info("skipping zero-length leaf-variant branch cleanup: nil entry in processor list")
			continue
		}
		p := e.GetParent()
		if p == nil {
			log.V(logger.VDebug).Info("skipping zero-length leaf-variant branch cleanup: entry has no parent (sync tree root)",
				"path", e.SdcpbPath().ToXPath(false))
			continue
		}
		if err := ops.DeleteBranch(ctx, p, &sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem(e.PathName(), nil)}}, consts.RunningIntentName); err != nil {
			return err
		}
	}

	if err := d.reconcilePendingCaptures(ctx, syncStartedAt, deletes, importedPaths); err != nil {
		return err
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

	// create a deep copy of the sync tree for revert operation
	syncTreeCopy, err := d.syncTree.DeepCopy(ctx)
	if err != nil {
		return err
	}

	// release the sync tree lock early, it is no longer needed
	syncTreeUnlock()

	// perform the revert operation to apply changes to the device
	// TODO: this should probably be executed in a separate goroutine
	err = d.performRevert(ctx, syncTreeCopy)
	if err != nil {
		return err
	}

	return nil

}

// pendingImportProbe tracks temporary Running state across a single ImportConfig
// call so ApplyToRunning can tell whether an incremental importer touched a
// causal pending path. Callers of beginPendingImportProbes must always invoke
// finishPendingImportProbes before RemoveDeletedProcessor runs, even on import error.
type pendingImportProbe struct {
	key     canonicalLeafPath
	path    *sdcpb.Path
	running *api.LeafEntry
}

// beginPendingImportProbes prepares observation probes on d.syncTree for causal
// pending paths not already covered by syncPaths; callers must hold syncTreeMutex.
func (d *Datastore) beginPendingImportProbes(ctx context.Context, syncStartedAt int64, syncPaths []*sdcpb.Path) []pendingImportProbe {
	var probes []pendingImportProbe
	for _, pending := range d.confirmedApplied.CausalCaptures() {
		if !pending.IsPending() || syncStartedAt < pending.captureAfterNano || pathCoveredBySync(pending.path, syncPaths) {
			continue
		}
		var running *api.LeafEntry
		entry, err := ops.NavigateSdcpbPath(ctx, d.syncTree.Entry, pending.path)
		if err == nil && entry != nil {
			if r := entry.GetLeafVariants().GetByOwner(consts.RunningIntentName); r != nil {
				if r.GetDeleteFlag() {
					continue
				}
				r.MarkDelete(false)
				running = r
			}
		}
		probes = append(probes, pendingImportProbe{
			key:     newCanonicalLeafPath(pending.path),
			path:    pending.path,
			running: running,
		})
	}
	return probes
}

// finishPendingImportProbes restores temporary probe markers on untouched Running
// leaves and returns the canonical paths observed during ImportConfig.
func (d *Datastore) finishPendingImportProbes(ctx context.Context, probes []pendingImportProbe) map[canonicalLeafPath]bool {
	if len(probes) == 0 {
		return nil
	}
	observed := make(map[canonicalLeafPath]bool, len(probes))
	for _, probe := range probes {
		if probe.running != nil {
			if !probe.running.GetDeleteFlag() {
				observed[probe.key] = true
			} else {
				probe.running.DropDeleteFlag()
			}
			continue
		}
		entry, err := ops.NavigateSdcpbPath(ctx, d.syncTree.Entry, probe.path)
		if err == nil && entry != nil && entry.GetLeafVariants().GetByOwner(consts.RunningIntentName) != nil {
			observed[probe.key] = true
		}
	}
	return observed
}

func (d *Datastore) reconcilePendingCaptures(ctx context.Context, syncStartedAt int64, syncPaths []*sdcpb.Path, importedPaths map[canonicalLeafPath]bool) error {
	for _, pending := range d.confirmedApplied.CausalCaptures() {
		if syncStartedAt < pending.captureAfterNano {
			restoreValue := pending.intendedValue
			if pending.confirmedValue != nil {
				restoreValue = pending.confirmedValue
			}
			if err := d.syncTree.AddUpdatesRecursive(ctx, []*treetypes.PathAndUpdate{
				treetypes.NewPathAndUpdate(
					pending.path,
					treetypes.NewUpdate(nil, restoreValue, consts.RunningValuesPrio, consts.RunningIntentName, 0),
				),
			}, treetypes.NewUpdateInsertFlags()); err != nil {
				return err
			}
			continue
		}
		if !pending.IsPending() {
			continue
		}

		wasObserved := pathCoveredBySync(pending.path, syncPaths) || importedPaths[newCanonicalLeafPath(pending.path)]
		if !wasObserved {
			continue
		}
		var observed *sdcpb.TypedValue
		entry, err := ops.NavigateSdcpbPath(ctx, d.syncTree.Entry, pending.path)
		if err == nil && entry != nil {
			if running := entry.GetLeafVariants().GetByOwner(consts.RunningIntentName); running != nil {
				observed = running.Value()
			}
		} else if err != nil && !errors.Is(err, ops.ErrNavigateSdcpbPathNotFound) {
			return err
		}
		d.confirmedApplied.CompletePending(pending, observed)
	}
	return nil
}

func pathCoveredBySync(path *sdcpb.Path, syncPaths []*sdcpb.Path) bool {
	path = canonicalPath(path)
	for _, syncPath := range syncPaths {
		if canonicalPath(syncPath).IsParentPathOf(path) {
			return true
		}
	}
	return false
}

func (d *Datastore) NewEmptyTree(ctx context.Context) (*tree.RootEntry, error) {
	tc := d.newTreeContext()
	newTree, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		return nil, err
	}
	return newTree, nil
}

func (d *Datastore) performRevert(ctx context.Context, t *tree.RootEntry) error {
	log := logger.FromContext(ctx)
	_, err := d.LoadAllButRunningIntents(ctx, t)
	if err != nil {
		return err
	}

	err = t.FinishInsertionPhase(ctx)
	if err != nil {
		return err
	}

	// TODO: optimize by checking only paths that where covered by the syncconfig
	del, err := t.GetDeletes(true)
	if err != nil {
		return err
	}
	updates := t.GetHighestPrecedence(true)

	// if we have deletes, we need to perform an apply
	performApply := len(del) > 0

	// if no deletes, check if we have updates
	if !performApply {
		performApply = len(updates) > 0
	}

	if performApply {
		// Skip Sync Revert's Southbound apply while a Northbound Transaction
		// (or another set-path holder) owns dmutex. Acting on this cycle's
		// possibly mid-flight device snapshot would race the in-flight Set;
		// the next sync cycle reevaluates with a fresh GET.
		if !d.dmutex.TryLock() {
			log.V(logger.VDebug).Info("skipping revert after sync: datastore locked")
			return nil
		}
		defer d.dmutex.Unlock()

		log.Info("reverting after sync")
		resp, err := d.applyIntent(ctx, adapter.NewEntryOutputAdapter(t.Entry))
		if err != nil {
			log.Error(err, "failed applying deviations to running", "response", utils.ProtoJSON(resp))
			return nil
		}
		setDoneUnixNano := time.Now().UnixNano()
		mutateOnWritePaths := d.collectPushedMutateOnWritePaths(updates)
		if len(mutateOnWritePaths) > 0 {
			readbacks := d.fetchMutateOnWriteReadbacks(ctx, mutateOnWritePaths)
			if err := d.applyReadbacksAndWriteBackRunning(ctx, updates, del, readbacks, setDoneUnixNano); err != nil {
				return err
			}
		} else {
			d.reconcilePlainStringRevertPushes(updates, del, setDoneUnixNano)
		}
	}

	return nil
}
