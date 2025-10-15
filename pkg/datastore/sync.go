package datastore

import (
	"context"

	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/importer/proto"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	log "github.com/sirupsen/logrus"
)

func (d *Datastore) ApplyToRunning(ctx context.Context, i *tree_persist.Intent) error {

	i.IntentName = tree.RunningIntentName
	i.Priority = tree.RunningValuesPrio
	i.Deviation = false

	d.syncTreeMutex.Lock()
	defer d.syncTreeMutex.Unlock()
	for _, delete := range i.ExplicitDeletes {
		//TODO this will most likely give us errors in case optimisticWriteback already deleted the entries.
		err := d.syncTree.DeleteBranch(ctx, delete, i.IntentName)
		if err != nil {
			log.Errorf("error deleting paths from datastore sync tree: %v", err)
			return err
		}
	}

	// need to reset the explicit deletes, they carry the actual deletes that we need to delete.
	// the imported would otherwise add explicit deletes for these.
	i.ExplicitDeletes = nil

	if i.GetRoot() != nil {
		importer := proto.NewProtoTreeImporter(i)
		err := d.syncTree.ImportConfig(ctx, &sdcpb.Path{}, importer, i.GetIntentName(), i.GetPriority(), treetypes.NewUpdateInsertFlags())
		if err != nil {
			return err
		}
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
