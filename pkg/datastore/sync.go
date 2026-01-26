package datastore

import (
	"context"
	"errors"
	"sync"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/importer"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/encoding/protojson"
)

func (d *Datastore) ApplyToRunning(ctx context.Context, deletes []*sdcpb.Path, importer importer.ImportConfigAdapter) error {

	log := logger.FromContext(ctx)

	d.syncTreeMutex.Lock()
	syncTreeUnlock := sync.OnceFunc(d.syncTreeMutex.Unlock)

	defer syncTreeUnlock()

	// create a virtual task pool for delete operations
	deleteMarkerPool := d.taskPool.NewVirtualPool(pool.VirtualFailFast)
	for _, delete := range deletes {
		// navigate to delete path
		deleteRoot, err := d.syncTree.NavigateSdcpbPath(ctx, delete)
		if err != nil {
			log.Error(err, "failed navigating to delete path", "path", delete.ToXPath(false))
			continue
		}
		// apply delete marker, setting owner delete flag on running intent
		err = tree.NewOwnerDeleteMarker(tree.NewOwnerDeleteMarkerTaskConfig(tree.RunningIntentName, false)).Run(deleteRoot, deleteMarkerPool)
		if err != nil {
			log.Error(err, "failed applying delete to path", "path", delete.ToXPath(false))
			continue
		}
	}

	// close the delete marker pool for submission and wait
	deleteMarkerPool.CloseAndWait()
	err := deleteMarkerPool.FirstError()
	if err != nil {
		return err
	}

	// import new config if provided
	if importer != nil {
		err := d.syncTree.ImportConfig(ctx, &sdcpb.Path{}, importer, tree.RunningIntentName, tree.RunningValuesPrio, false, treetypes.NewUpdateInsertFlags())
		if err != nil {
			return err
		}
	}

	// create a virtual task pool for removeDeleted operations
	removeDeletedPool := d.taskPool.NewVirtualPool(pool.VirtualFailFast)

	// run remove deleted processor to clean up entries marked as deleted by owner
	delProcessorParams := tree.NewRemoveDeletedProcessorParameters(tree.RunningIntentName)
	err = tree.NewRemoveDeletedProcessor(delProcessorParams).Run(d.syncTree.GetRoot(), removeDeletedPool)
	if err != nil {
		return err
	}

	// close the remove deleted pool for submission and wait
	removeDeletedPool.CloseAndWait()
	err = errors.Join(removeDeletedPool.Errors()...)
	if err != nil {
		return err
	}

	// delete entries that have zero-length leaf variant entries after remove deleted processing
	for _, e := range delProcessorParams.GetZeroLengthLeafVariantEntries() {
		err := e.GetParent().DeleteBranch(ctx, &sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem(e.PathName(), nil)}}, tree.RunningIntentName)
		if err != nil {
			return err
		}
	}

	// conditional trace logging
	if log := log.V(logger.VTrace); log.Enabled() {
		treeExport, err := d.syncTree.TreeExport(tree.RunningIntentName, tree.RunningValuesPrio, false)
		if err == nil {
			json, err := protojson.MarshalOptions{Multiline: false}.Marshal(treeExport)
			if err == nil {
				log.Info("synctree after sync apply", "content", string(json))
			}
		}
	}

	// run reset flags processor to reset flags
	resetFlagsPool := d.taskPool.NewVirtualPool(pool.VirtualTolerant)
	resetFlagsProcessorParams := tree.NewResetFlagsProcessorParameters(true, true, true)
	err = tree.NewResetFlagsProcessor(resetFlagsProcessorParams).Run(d.syncTree.GetRoot(), resetFlagsPool)
	if err != nil {
		return err
	}

	// close the resetFlags pool for submission and wait
	resetFlagsPool.CloseAndWait()
	if errors.Join(resetFlagsPool.Errors()...) != nil {
		return err
	}

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

func (d *Datastore) NewEmptyTree(ctx context.Context) (*tree.RootEntry, error) {
	tc := tree.NewTreeContext(d.schemaClient, tree.RunningIntentName)
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

	// if we have deletes, we need to perform an apply
	performApply := len(del) > 0

	// if no deletes, check if we have updates
	if !performApply {
		updList, err := t.ToProtoUpdates(ctx, true)
		if err != nil {
			return err
		}
		// if the update list is non-empty, we need to perform an apply
		performApply = len(updList) > 0
	}

	if performApply {
		resp, err := d.applyIntent(ctx, t)
		if err != nil {
			respJ := protojson.MarshalOptions{Multiline: false}
			respStr, _ := respJ.Marshal(resp)
			log.Error(err, "failed applying deviations to running", "response", string(respStr))
		}
	}

	return nil
}
