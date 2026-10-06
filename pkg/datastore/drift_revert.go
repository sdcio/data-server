package datastore

import (
	"context"
	"errors"
	"fmt"

	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/api/adapter"
	"github.com/sdcio/data-server/pkg/tree/consts"
	treeproto "github.com/sdcio/data-server/pkg/tree/importer/proto"
	"github.com/sdcio/data-server/pkg/tree/ops"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
)

func (d *Datastore) setDriftRevertPaths(paths []*sdcpb.Path) {
	d.driftRevertPathsMu.Lock()
	defer d.driftRevertPathsMu.Unlock()
	d.driftRevertPaths = clonePaths(paths)
}

func (d *Datastore) driftRevertPathsSnapshot() []*sdcpb.Path {
	d.driftRevertPathsMu.Lock()
	defer d.driftRevertPathsMu.Unlock()
	return clonePaths(d.driftRevertPaths)
}

func clonePaths(in []*sdcpb.Path) []*sdcpb.Path {
	if len(in) == 0 {
		return nil
	}
	out := make([]*sdcpb.Path, len(in))
	for i, p := range in {
		if p != nil {
			out[i] = p.DeepCopy()
		}
	}
	return out
}

func (d *Datastore) performChangedPathRevert(ctx context.Context, syncRoot *tree.RootEntry, changedPaths, revertScopes []*sdcpb.Path, revertSnapshot *tree.RootEntry) error {
	log := logger.FromContext(ctx)

	revertPaths := changedPaths
	scopes := revertScopes
	if len(revertPaths) == 0 {
		revertPaths = d.driftRevertPathsSnapshot()
		scopes = ops.RevertScopesFromChangedPaths(revertPaths)
	}
	if len(revertPaths) == 0 {
		return d.performFullTreeRevert(ctx, syncRoot)
	}

	d.setDriftRevertPaths(revertPaths)

	revertTree := revertSnapshot
	var err error
	if revertTree == nil {
		revertTree, err = buildPartialRevertTree(ctx, syncRoot, scopes)
		if err != nil {
			return err
		}
	}

	if err := d.loadIntentsForScopes(ctx, revertTree, scopes); err != nil {
		return err
	}
	if err := revertTree.FinishInsertionPhase(ctx); err != nil {
		return err
	}

	del, err := revertTree.GetDeletes(true)
	if err != nil {
		return err
	}
	performApply := len(del) > 0
	if !performApply {
		updList, err := ops.ToProtoUpdates(ctx, revertTree.Entry, true)
		if err != nil {
			return err
		}
		performApply = len(updList) > 0
	}
	if !performApply {
		d.setDriftRevertPaths(nil)
		return nil
	}

	log.Info("reverting after sync")
	resp, applyErr := d.applyIntent(ctx, adapter.NewEntryOutputAdapter(revertTree.Entry))
	if applyErr != nil {
		log.Error(applyErr, "failed applying deviations to running", "response", utils.ProtoJSON(resp))
		return applyErr
	}
	d.setDriftRevertPaths(nil)
	return nil
}

func (d *Datastore) performFullTreeRevert(ctx context.Context, syncRoot *tree.RootEntry) error {
	syncTreeCopy, err := syncRoot.DeepCopy(ctx)
	if err != nil {
		return err
	}
	return d.performRevert(ctx, syncTreeCopy)
}

func buildPartialRevertTree(ctx context.Context, syncRoot *tree.RootEntry, scopes []*sdcpb.Path) (*tree.RootEntry, error) {
	syncTC, ok := syncRoot.GetTreeContext().(*tree.TreeContext)
	if !ok {
		return nil, fmt.Errorf("buildPartialRevertTree: unexpected tree context type")
	}
	revertTC := syncTC.DeepCopy().(*tree.TreeContext)
	revertRoot, err := tree.NewTreeRoot(ctx, revertTC)
	if err != nil {
		return nil, err
	}
	for _, scope := range scopes {
		if scope == nil {
			continue
		}
		if err := copyRunningScope(ctx, syncRoot.Entry, revertRoot.Entry, scope); err != nil {
			if errors.Is(err, ops.ErrNavigateSdcpbPathNotFound) {
				continue
			}
			return nil, err
		}
	}
	return revertRoot, nil
}

func copyRunningScope(ctx context.Context, syncRoot, revertRoot api.Entry, scope *sdcpb.Path) error {
	syncEntry, err := ops.NavigateSdcpbPath(ctx, syncRoot, scope)
	if err != nil {
		return err
	}
	revertEntry, err := ops.GetOrCreateChilds(ctx, revertRoot, scope)
	if err != nil {
		return err
	}
	return mirrorRunningSubtree(ctx, syncEntry, revertEntry)
}

func mirrorRunningSubtree(ctx context.Context, src, dst api.Entry) error {
	if ops.HoldsLeafVariants(src) {
		// leaves flagged deleted are removed from the sync tree by the remove-deleted
		// processor right after this snapshot; they must not appear in the revert tree.
		if le := src.GetLeafVariants().GetByOwner(consts.RunningIntentName); le != nil && !le.GetDeleteFlag() {
			runCopy := le.DeepCopy(dst)
			// the sync reset-flags pass would have cleared these; Running-only changes
			// must not count as updates to push back to the device.
			runCopy.IsNew = false
			runCopy.IsUpdated = false
			dst.GetLeafVariants().Add(runCopy)
		}
	}
	for _, name := range src.GetChildMap().SortedKeys() {
		childSrc, ok := src.GetChildMap().GetEntry(name)
		if !ok {
			continue
		}
		if !hasLiveRunning(childSrc) {
			continue
		}
		childDst, err := api.NewEntry(ctx, dst, name, dst.GetTreeContext())
		if err != nil {
			return err
		}
		if err := mirrorRunningSubtree(ctx, childSrc, childDst); err != nil {
			return err
		}
	}
	return nil
}

// hasLiveRunning reports whether the subtree holds any Running leaf not flagged deleted.
func hasLiveRunning(e api.Entry) bool {
	if ops.HoldsLeafVariants(e) {
		if le := e.GetLeafVariants().GetByOwner(consts.RunningIntentName); le != nil && !le.GetDeleteFlag() {
			return true
		}
	}
	for _, name := range e.GetChildMap().SortedKeys() {
		if c, ok := e.GetChildMap().GetEntry(name); ok && hasLiveRunning(c) {
			return true
		}
	}
	return false
}

func (d *Datastore) loadIntentsForScopes(ctx context.Context, root *tree.RootEntry, scopes []*sdcpb.Path) error {
	log := logger.FromContext(ctx)
	intentChan := make(chan *tree_persist.Intent)
	errChan := make(chan error, 1)

	go d.cacheClient.IntentGetAll(ctx, []string{consts.RunningIntentName}, intentChan, errChan)

	for {
	selectLoop:
		select {
		case err, ok := <-errChan:
			if !ok {
				errChan = nil
				break selectLoop
			}
			return err
		case <-ctx.Done():
			return fmt.Errorf("context closed while retrieving intents for drift revert")
		case intent, ok := <-intentChan:
			if !ok {
				intentChan = nil
				break selectLoop
			}
			if !intentRelevantToScopes(intent, scopes) {
				continue
			}
			filtered := treeproto.FilterIntentForScopes(intent, scopes)
			log.V(logger.VDebug).Info("adding intent to partial revert tree", "intent", filtered.GetIntentName())
			protoLoader := treeproto.NewProtoTreeImporter(filtered)
			if _, err := root.ImportConfig(ctx, nil, protoLoader, treetypes.NewUpdateInsertFlags(), d.taskPool); err != nil {
				return err
			}
		}
		if errChan == nil && intentChan == nil {
			return nil
		}
	}
}

func intentRelevantToScopes(intent *tree_persist.Intent, scopes []*sdcpb.Path) bool {
	if intent == nil {
		return false
	}
	for _, del := range intent.GetExplicitDeletes() {
		if del == nil {
			continue
		}
		if len(del.GetElem()) == 0 {
			return true
		}
		for _, scope := range scopes {
			if scope != nil && (del.IsParentPathOf(scope) || scope.IsParentPathOf(del) || del.PathsEqual(scope)) {
				return true
			}
		}
	}
	if intent.GetRoot() == nil {
		return false
	}
	filtered := treeproto.FilterIntentForScopes(intent, scopes)
	return filtered.GetRoot() != nil && (len(filtered.GetRoot().GetChilds()) > 0 || len(filtered.GetRoot().GetLeafVariant()) > 0)
}
