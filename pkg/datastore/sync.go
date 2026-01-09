package datastore

import (
	"context"

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
	defer d.syncTreeMutex.Unlock()
	for _, delete := range deletes {
		err := d.syncTree.DeleteBranch(ctx, delete, tree.RunningIntentName)
		if err != nil {
			log.Error(err, "failed deleting path from datastore sync tree", "severity", "WARN", "path", delete.ToXPath(false))
			continue
		}
	}

	if importer != nil {
		err := d.syncTree.ImportConfig(ctx, &sdcpb.Path{}, importer, tree.RunningIntentName, tree.RunningValuesPrio, treetypes.NewUpdateInsertFlags())
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
