package tree

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/processors"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// RootEntry the root of the cache.Update tree
type RootEntry struct {
	api.Entry
}

// NewTreeRoot Instantiate a new Tree Root element.
func NewTreeRoot(ctx context.Context, tc api.TreeContext) (*RootEntry, error) {
	sea, err := NewSharedEntryAttributes(ctx, nil, "", tc)
	if err != nil {
		return nil, err
	}

	root := &RootEntry{
		Entry: sea,
	}

	return root, nil
}

// stringToDisk is kept for ad-hoc debugging dumps.
//
//nolint:unused // Debug helper intentionally left in place.
func (r *RootEntry) stringToDisk(filename string) error {
	err := os.WriteFile(filename, []byte(r.String()), 0755)
	return err
}

func (r *RootEntry) DeepCopy(ctx context.Context) (*RootEntry, error) {
	tc := r.GetTreeContext().DeepCopy()
	se, err := r.Entry.DeepCopy(tc, nil)
	if err != nil {
		return nil, err
	}

	result := &RootEntry{
		Entry: se,
	}

	return result, nil
}

func (r *RootEntry) AddUpdatesRecursive(ctx context.Context, us []*types.PathAndUpdate, flags *types.UpdateInsertFlags) error {
	var err error
	for idx, u := range us {
		_ = idx
		_, err = ops.AddUpdateRecursive(ctx, r.Entry, u.GetPath(), u.GetUpdate(), flags)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RootEntry) ImportConfig(ctx context.Context, basePath *sdcpb.Path, importer importer.ImportConfigAdapter, flags *types.UpdateInsertFlags, poolFactory pool.VirtualPoolFactory) (*types.ImportStats, error) {
	e, err := ops.GetOrCreateChilds(ctx, r.Entry, basePath)
	if err != nil {
		return nil, err
	}
	ImportConfigProcessor := processors.NewImportConfigProcessor(importer, flags)
	err = ImportConfigProcessor.Run(ctx, e, poolFactory)
	if err != nil {
		return nil, err
	}
	return ImportConfigProcessor.GetStats(), nil
}

func (r *RootEntry) SetNonRevertiveIntent(intentName string, nonRevertive bool) {
	r.GetTreeContext().NonRevertiveInfo().Add(intentName, nonRevertive)
}

// String returns the string representation of the Tree.
func (r *RootEntry) String() string {
	s := []string{}
	s = r.Entry.StringIndent(s)
	return strings.Join(s, "\n")
}

// GetUpdatesForOwner returns the updates that have been calculated for the given intent / owner
func (r *RootEntry) GetUpdatesForOwner(owner string) types.UpdateSlice {
	// retrieve all the entries from the tree that belong to the given
	// Owner / Intent, skipping the once marked for deletion
	// this is to insert / update entries in the cache.
	return api.LeafEntriesToUpdates(ops.LeafsOfOwner(r.Entry, owner, api.FilterNonDeletedButNewOrUpdated))
}

// GetDeletesForOwner returns the deletes that have been calculated for the given intent / owner
func (r *RootEntry) GetDeletesForOwner(owner string) sdcpb.Paths {
	// retrieve all entries from the tree that belong to the given user
	// and that are marked for deletion.
	// This is to cover all the cases where an intent was changed and certain
	// part of the config got deleted.
	deletesOwnerUpdates := api.LeafEntriesToUpdates(ops.LeafsOfOwner(r.Entry, owner, api.FilterDeleted))
	// they are retrieved as cache.update, we just need the path for deletion from cache
	deletesOwner := make(sdcpb.Paths, 0, len(deletesOwnerUpdates))
	// so collect the paths
	for _, d := range deletesOwnerUpdates {
		deletesOwner = append(deletesOwner, d.SdcpbPath())
	}
	return deletesOwner
}

// GetHighesPrecedence return the new cache.Update entried from the tree that are the highes priority.
// If the onlyNewOrUpdated option is set to true, only the New or Updated entries will be returned
// It will append to the given list and provide a new pointer to the slice
func (r *RootEntry) GetHighestPrecedence(onlyNewOrUpdated bool) api.LeafVariantSlice {
	return ops.GetHighestPrecedence(r.Entry, onlyNewOrUpdated, false, false)
}

// GetDeletes returns the paths that due to the Tree content are to be deleted from the southbound device.
func (r *RootEntry) GetDeletes(aggregatePaths bool) (types.DeleteEntriesList, error) {
	return ops.GetDeletes(r.Entry, aggregatePaths)
}

func (r *RootEntry) GetAncestorSchema() (*sdcpb.SchemaElem, int) {
	return nil, 0
}

// DeleteSubtree Deletes from the tree, all elements of the PathSlice defined branch of the given owner. Return values are remainsToExist and error if an error occured.
func (r *RootEntry) DeleteBranchPaths(ctx context.Context, deletes types.DeleteEntriesList, intentName string) error {
	for _, del := range deletes {
		err := ops.DeleteBranch(ctx, r.Entry, del.SdcpbPath(), intentName)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RootEntry) FinishInsertionPhase(ctx context.Context) error {
	log := logf.FromContext(ctx)
	edpsc := processors.ExplicitDeleteProcessorStatCollection{}

	// apply the explicit deletes
	for deletePathPrio := range r.GetTreeContext().ExplicitDeletes().Items() {

		params := processors.NewExplicitDeleteTaskParameters(deletePathPrio.GetOwner(), deletePathPrio.GetPrio())

		for path := range deletePathPrio.PathItems() {

			// set the priority
			// navigate to the stated path
			entry, err := ops.NavigateSdcpbPath(ctx, r.Entry, path)
			if err != nil {
				log.Error(nil, "Applying explicit delete - path not found, skipping", "severity", "WARN", "path", path.ToXPath(false))
			}
			edp := processors.NewExplicitDeleteProcessor(params)
			err = edp.Run(ctx, entry, r.GetTreeContext().PoolFactory())
			if err != nil {
				return err
			}
			edpsc[deletePathPrio.GetOwner()] = edp
		}
	}
	// conditional logging
	if edpsc.ContainsEntries() {
		log.V(logf.VDebug).Info("ExplicitDeletes added", "explicit-deletes", utils.MapToString(edpsc.Stats(), ", ", func(k string, v int) string {
			return fmt.Sprintf("%s=%d", k, v)
		}))
	}

	return r.Entry.FinishInsertionPhase(ctx)
}
