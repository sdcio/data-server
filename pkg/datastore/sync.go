package datastore

import (
	"context"

	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/importer/proto"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
)

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
