package datastore

import (
	"context"

	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/importer/proto"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
)

// func (d *Datastore) SyncOld(ctx context.Context) {
// 	go d.sbi.Sync(ctx,
// 		d.config.Sync,
// 	)

// 	var err error
// 	var startTs int64

// 	d.syncTreeCandidate, err = tree.NewTreeRoot(ctx, tree.NewTreeContext(d.schemaClient, tree.RunningIntentName))
// 	if err != nil {
// 		log.Errorf("creating a new synctree candidate: %v", err)
// 		return
// 	}

// 	for {
// 		select {
// 		case <-ctx.Done():
// 			if !errors.Is(ctx.Err(), context.Canceled) {
// 				log.Errorf("datastore %s sync stopped: %v", d.Name(), ctx.Err())
// 			}
// 			return
// 		case syncup := <-d.synCh:
// 			switch {
// 			case syncup.Start:
// 				log.Debugf("%s: sync start", d.Name())
// 				startTs = time.Now().Unix()

// 			case syncup.End:
// 				log.Debugf("%s: sync end", d.Name())

// 				startTs = 0

// 				d.syncTreeMutex.Lock()
// 				d.syncTree = d.syncTreeCandidate
// 				d.syncTreeMutex.Unlock()

// 				// create new syncTreeCandidat
// 				d.syncTreeCandidate, err = tree.NewTreeRoot(ctx, tree.NewTreeContext(d.schemaClient, tree.RunningIntentName))
// 				if err != nil {
// 					log.Errorf("creating a new synctree candidate: %v", err)
// 					return
// 				}

// 				// export and write to cache
// 				runningExport, err := d.syncTree.TreeExport(tree.RunningIntentName, tree.RunningValuesPrio, false)
// 				if err != nil {
// 					log.Error(err)
// 					continue
// 				}
// 				err = d.cacheClient.IntentModify(ctx, runningExport)
// 				if err != nil {
// 					log.Errorf("issue modifying running cache content: %v", err)
// 					continue
// 				}
// 			default:
// 				if startTs == 0 {
// 					startTs = time.Now().Unix()
// 				}
// 				err := d.writeToSyncTreeCandidate(ctx, syncup.Update.GetUpdate(), startTs)
// 				if err != nil {
// 					log.Errorf("failed to write to sync tree: %v", err)
// 				}
// 			}
// 		}
// 	}
// }

func (d *Datastore) writeToSyncTreeCandidate(ctx context.Context, updates []*sdcpb.Update, ts int64) error {
	upds, err := treetypes.ExpandAndConvertIntent(ctx, d.schemaClient, tree.RunningIntentName, tree.RunningValuesPrio, updates, ts)
	if err != nil {
		return err
	}

	for idx, upd := range upds {
		_ = idx
		_, err := d.syncTreeCandidate.AddUpdateRecursive(ctx, upd.Path(), upd, treetypes.NewUpdateInsertFlags())
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Datastore) ApplyToRunning(ctx context.Context, i *tree_persist.Intent) error {

	i.IntentName = tree.RunningIntentName
	i.Priority = tree.RunningValuesPrio
	i.Deviation = false

	importer := proto.NewProtoTreeImporter(i)

	// need to reset the explicit deletes, they carry the actual deletes that we need to delete.
	// the imported would otherwise add explicit deletes for these.
	deletes := i.ExplicitDeletes
	i.ExplicitDeletes = nil

	d.syncTreeMutex.Lock()
	defer d.syncTreeMutex.Unlock()
	for _, delete := range deletes {
		err := d.syncTree.DeleteBranch(ctx, delete, i.IntentName)
		if err != nil {
			return err
		}
	}

	return d.syncTree.ImportConfig(ctx, &sdcpb.Path{}, importer, i.GetIntentName(), i.GetPriority(), treetypes.NewUpdateInsertFlags())
}

func (d *Datastore) NewEmptyTree(ctx context.Context) (*tree.RootEntry, error) {
	tc := tree.NewTreeContext(d.schemaClient, tree.RunningIntentName)
	newTree, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		return nil, err
	}
	return newTree, nil
}
