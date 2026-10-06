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
	s := r.StringIndent(nil)
	return strings.Join(s, "\n")
}

// StringExpanded returns debug output with delete-path coverage expanded into
// its effective per-leaf explicit-delete variants.
func (r *RootEntry) StringExpanded() string {
	return strings.Join(r.stringIndentExpanded(nil), "\n")
}

func (r *RootEntry) stringIndentExpanded(result []string) []string {
	var walk func(api.Entry, []string) []string
	walk = func(entry api.Entry, lines []string) []string {
		indent := strings.Repeat("  ", entry.GetLevel())
		lines = append(lines, indent+entry.PathName())

		for _, coverage := range entry.GetDeletePathCoverages() {
			lines = append(lines, fmt.Sprintf("%s -> Owner: %s, Priority: %d, explicit delete, covers subtree",
				indent, coverage.GetOwner(), coverage.GetPrio()))
		}
		for _, child := range entry.GetChildMap().GetAllSorted() {
			lines = walk(child, lines)
		}

		for _, leaf := range entry.GetLeafVariants().EffectiveItems() {
			lines = append(lines, fmt.Sprintf("%s -> %s", indent, leaf.String()))
		}
		return lines
	}
	return walk(r.Entry, result)
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
	if r.Entry == nil {
		return fmt.Errorf("DeleteBranchPaths: nil root entry")
	}
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
	coverageCount := map[string]int{}
	for oldCoverage := range r.GetTreeContext().DeletePathCoverage().Items() {
		for path := range oldCoverage.PathItems() {
			if entry, err := ops.NavigateSdcpbPath(ctx, r.Entry, path); err == nil {
				entry.SetDeletePathCoverages(nil)
			}
		}
	}
	r.GetTreeContext().ResetDeletePathCoverage()
	coverage := api.NewDeletePaths()

	// Validate and retain explicit deletes as branch coverage. Effective
	// per-leaf variants are evaluated lazily by LeafVariants.
	for deletePathPrio := range r.GetTreeContext().ExplicitDeletes().Items() {
		for path := range deletePathPrio.PathItems() {
			_, err := ops.NavigateSdcpbPath(ctx, r.Entry, path)
			if err != nil {
				log.Error(nil, "Applying explicit delete - path not found, skipping", "severity", "WARN", "path", path.ToXPath(false))
				continue
			}
			coverage.AddPath(deletePathPrio.GetOwner(), deletePathPrio.GetPrio(), path)
			coverageCount[deletePathPrio.GetOwner()]++
		}
	}
	r.GetTreeContext().SetDeletePathCoverage(coverage)
	for deletePathPrio := range coverage.Items() {
		for path := range deletePathPrio.PathItems() {
			entry, err := ops.NavigateSdcpbPath(ctx, r.Entry, path)
			if err != nil {
				return err
			}
			entry.SetDeletePathCoverages(append(entry.GetDeletePathCoverages(), deletePathPrio))
		}
	}
	if err := r.Entry.FinishInsertionPhase(ctx); err != nil {
		return err
	}

	if len(coverageCount) > 0 {
		log.V(logf.VDebug).Info("Explicit delete coverage added", "explicit-delete-paths", coverageCount)
	}

	return nil
}
