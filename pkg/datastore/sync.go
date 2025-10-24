package datastore

import (
	"context"

	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/importer"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
)

func (d *Datastore) ApplyToRunning(ctx context.Context, deletes []*sdcpb.Path, importer importer.ImportConfigAdapter) error {

	d.syncTreeMutex.Lock()
	defer d.syncTreeMutex.Unlock()
	for _, delete := range deletes {
		//TODO this will most likely give us errors in case optimisticWriteback already deleted the entries.
		err := d.syncTree.DeleteBranch(ctx, delete, tree.RunningIntentName)
		if err != nil {
			log.Errorf("error deleting paths from datastore sync tree: %v", err)
			return err
		}
	}

	if importer != nil {
		err := d.syncTree.ImportConfig(ctx, &sdcpb.Path{}, importer, tree.RunningIntentName, tree.RunningValuesPrio, treetypes.NewUpdateInsertFlags())
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
