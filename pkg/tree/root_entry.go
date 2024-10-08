package tree

import (
	"context"
	"strings"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// RootEntry the root of the cache.Update tree
type RootEntry struct {
	*sharedEntryAttributes
}

// NewTreeRoot Instantiate a new Tree Root element.
func NewTreeRoot(ctx context.Context, tc *TreeContext) (*RootEntry, error) {
	sea, err := newSharedEntryAttributes(ctx, nil, "", tc)
	if err != nil {
		return nil, err
	}

	root := &RootEntry{
		sharedEntryAttributes: sea,
	}

	err = tc.SetRoot(sea)
	if err != nil {
		return nil, err
	}

	return root, nil
}

func (r *RootEntry) LoadIntendedStoreOwnerData(ctx context.Context, owner string, pathKeySet *PathSet) {
	tc := r.getTreeContext()
	ownerPaths := tc.GetPathsOfOwner(owner)

	// add the given paths as well
	ownerPaths.Join(pathKeySet)

	// Get all entries of the already existing intent
	highesCurrentCacheEntries := tc.ReadCurrentUpdatesHighestPriorities(ctx, ownerPaths.GetPaths(), 2)

	// add all the existing entries
	for _, entry := range highesCurrentCacheEntries {
		r.AddCacheUpdateRecursive(ctx, entry, false)
	}

	// Mark all the entries that belong to the owner / intent as deleted.
	// This is to allow for intent updates. We mark all existing entries for deletion up front.
	r.markOwnerDelete(owner)
}

// String returns the string representation of the Tree.
func (r *RootEntry) String() string {
	s := []string{}
	s = r.sharedEntryAttributes.StringIndent(s)
	return strings.Join(s, "\n")
}

// GetUpdatesForOwner returns the updates that have been calculated for the given intent / owner
func (r *RootEntry) GetUpdatesForOwner(owner string) UpdateSlice {
	// retrieve all the entries from the tree that belong to the given
	// Owner / Intent, skipping the once marked for deletion
	// this is to insert / update entries in the cache.
	return LeafEntriesToCacheUpdates(r.getByOwnerFiltered(owner, FilterNonDeletedButNewOrUpdated))
}

// GetDeletesForOwner returns the deletes that have been calculated for the given intent / owner
func (r *RootEntry) GetDeletesForOwner(owner string) PathSlices {
	// retrieve all entries from the tree that belong to the given user
	// and that are marked for deletion.
	// This is to cover all the cases where an intent was changed and certain
	// part of the config got deleted.
	deletesOwnerUpdates := LeafEntriesToCacheUpdates(r.getByOwnerFiltered(owner, FilterDeleted))
	// they are retrieved as cache.update, we just need the path for deletion from cache
	deletesOwner := make(PathSlices, 0, len(deletesOwnerUpdates))
	// so collect the paths
	for _, d := range deletesOwnerUpdates {
		deletesOwner = append(deletesOwner, d.GetPath())
	}
	return deletesOwner
}

// GetHighesPrecedence return the new cache.Update entried from the tree that are the highes priority.
// If the onlyNewOrUpdated option is set to true, only the New or Updated entries will be returned
// It will append to the given list and provide a new pointer to the slice
func (r *RootEntry) GetHighestPrecedence(onlyNewOrUpdated bool) LeafVariantSlice {
	return r.sharedEntryAttributes.GetHighestPrecedence(make(LeafVariantSlice, 0), onlyNewOrUpdated)
}

// GetDeletes returns the paths that due to the Tree content are to be deleted from the southbound device.
func (r *RootEntry) GetDeletes(aggregatePaths bool) PathSlices {
	deletes := PathSlices{}
	return r.sharedEntryAttributes.GetDeletes(deletes, aggregatePaths)
}

// getTreeContext returns the handle to the TreeContext
func (r *RootEntry) getTreeContext() *TreeContext {
	return r.treeContext
}

func (r *RootEntry) GetAncestorSchema() (*sdcpb.SchemaElem, int) {
	return nil, 0
}

// getByOwnerFiltered returns the Tree content filtered by owner, whilst allowing to filter further
// via providing additional LeafEntryFilter
func (r *RootEntry) getByOwnerFiltered(owner string, f ...LeafEntryFilter) []*LeafEntry {
	result := []*LeafEntry{}
	// retrieve all leafentries for the owner
	leafEntries := r.sharedEntryAttributes.GetByOwner(owner, result)
	// range through entries
NEXTELEMENT:
	for _, e := range leafEntries {
		// apply filter
		for _, filter := range f {
			// if the filter yields false, skip
			if !filter(e) {
				continue NEXTELEMENT
			}
		}
		result = append(result, e)
	}
	return result
}
