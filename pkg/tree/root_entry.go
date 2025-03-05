package tree

import (
	"context"
	"fmt"
	"strings"

	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/tree/tree_persist"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// RootEntry the root of the cache.Update tree
type RootEntry struct {
	*sharedEntryAttributes
}

var (
	ErrorIntentNotPresent = fmt.Errorf("intent not present")
)

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

func (r *RootEntry) DeepCopy(ctx context.Context) (*RootEntry, error) {
	tc := r.treeContext.deepCopy()
	se, err := r.sharedEntryAttributes.deepCopy(tc, nil)

	result := &RootEntry{
		sharedEntryAttributes: se,
	}

	err = tc.SetRoot(result.sharedEntryAttributes)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *RootEntry) AddCacheUpdatesRecursive(ctx context.Context, us types.UpdateSlice, flags *Flags) error {
	var err error
	for _, u := range us {
		_, err = r.sharedEntryAttributes.AddUpdateRecursive(ctx, u, flags)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RootEntry) ImportConfig(ctx context.Context, t importer.ImportConfigAdapter, intentName string, intentPrio int32, flags *Flags) error {
	r.treeContext.SetActualOwner(intentName)
	return r.sharedEntryAttributes.ImportConfig(ctx, t, intentName, intentPrio, flags)
}

func (r *RootEntry) Validate(ctx context.Context, concurrent bool) types.ValidationResults {
	// perform validation
	// we use a channel and cumulate all the errors
	validationResultEntryChan := make(chan *types.ValidationResultEntry, 10)

	// start validation in a seperate goroutine
	go func() {
		r.sharedEntryAttributes.Validate(ctx, validationResultEntryChan, concurrent)
		close(validationResultEntryChan)
	}()

	// create a ValidationResult struct
	validationResult := types.ValidationResults{}

	// read from the validationResult channel
	for e := range validationResultEntryChan {
		validationResult.AddEntry(e)
	}

	return validationResult
}

// String returns the string representation of the Tree.
func (r *RootEntry) String() string {
	s := []string{}
	s = r.sharedEntryAttributes.StringIndent(s)
	return strings.Join(s, "\n")
}

// GetUpdatesForOwner returns the updates that have been calculated for the given intent / owner
func (r *RootEntry) GetUpdatesForOwner(owner string) types.UpdateSlice {
	// retrieve all the entries from the tree that belong to the given
	// Owner / Intent, skipping the once marked for deletion
	// this is to insert / update entries in the cache.
	return LeafEntriesToUpdates(r.getByOwnerFiltered(owner, FilterNonDeletedButNewOrUpdated))
}

// GetDeletesForOwner returns the deletes that have been calculated for the given intent / owner
func (r *RootEntry) GetDeletesForOwner(owner string) types.PathSlices {
	// retrieve all entries from the tree that belong to the given user
	// and that are marked for deletion.
	// This is to cover all the cases where an intent was changed and certain
	// part of the config got deleted.
	deletesOwnerUpdates := LeafEntriesToUpdates(r.getByOwnerFiltered(owner, FilterDeleted))
	// they are retrieved as cache.update, we just need the path for deletion from cache
	deletesOwner := make(types.PathSlices, 0, len(deletesOwnerUpdates))
	// so collect the paths
	for _, d := range deletesOwnerUpdates {
		deletesOwner = append(deletesOwner, d.GetPathSlice())
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
func (r *RootEntry) GetDeletes(aggregatePaths bool) (types.DeleteEntriesList, error) {
	deletes := []types.DeleteEntry{}
	return r.sharedEntryAttributes.GetDeletes(deletes, aggregatePaths)
}

func (r *RootEntry) GetAncestorSchema() (*sdcpb.SchemaElem, int) {
	return nil, 0
}

func (r *RootEntry) TreeExport(owner string, priority int32) (*tree_persist.Intent, error) {
	te, err := r.sharedEntryAttributes.TreeExport(owner)
	if err != nil {
		return nil, err
	}
	if te != nil {
		return &tree_persist.Intent{
			IntentName: owner,
			Root:       te[0],
			Priority:   priority,
		}, nil
	}
	return nil, ErrorIntentNotPresent
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

func (r *RootEntry) DeleteSubtreePaths(deletes types.DeleteEntriesList, intentName string) {
	for _, del := range deletes {
		r.DeleteSubtree(del.Path(), intentName)
	}
}
