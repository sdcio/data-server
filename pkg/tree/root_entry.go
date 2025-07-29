package tree

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sdcio/data-server/pkg/config"
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

// stringToDisk just for debugging purpose
func (r *RootEntry) stringToDisk(filename string) error {
	err := os.WriteFile(filename, []byte(r.String()), 0755)
	return err
}

func (r *RootEntry) DeepCopy(ctx context.Context) (*RootEntry, error) {
	tc := r.treeContext.deepCopy()
	se, err := r.sharedEntryAttributes.deepCopy(tc, nil)
	if err != nil {
		return nil, err
	}

	result := &RootEntry{
		sharedEntryAttributes: se,
	}

	err = tc.SetRoot(result.sharedEntryAttributes)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *RootEntry) AddUpdatesRecursive(ctx context.Context, us types.UpdateSlice, flags *types.UpdateInsertFlags) error {
	var err error
	for idx, u := range us {
		_ = idx
		_, err = r.sharedEntryAttributes.AddUpdateRecursive(ctx, u.Path().DeepCopy(), u, flags)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RootEntry) ImportConfig(ctx context.Context, basePath *sdcpb.Path, importer importer.ImportConfigAdapter, intentName string, intentPrio int32, flags *types.UpdateInsertFlags) error {
	r.treeContext.SetActualOwner(intentName)

	e, err := r.sharedEntryAttributes.getOrCreateChilds(ctx, basePath)
	if err != nil {
		return err
	}

	return e.ImportConfig(ctx, importer, intentName, intentPrio, flags)
}

func (r *RootEntry) Validate(ctx context.Context, vCfg *config.Validation) types.ValidationResults {
	// perform validation
	// we use a channel and cumulate all the errors
	validationResultEntryChan := make(chan *types.ValidationResultEntry, 10)

	// start validation in a seperate goroutine
	go func() {
		r.sharedEntryAttributes.Validate(ctx, validationResultEntryChan, vCfg)
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
func (r *RootEntry) GetDeletesForOwner(owner string) sdcpb.Paths {
	// retrieve all entries from the tree that belong to the given user
	// and that are marked for deletion.
	// This is to cover all the cases where an intent was changed and certain
	// part of the config got deleted.
	deletesOwnerUpdates := LeafEntriesToUpdates(r.getByOwnerFiltered(owner, FilterDeleted))
	// they are retrieved as cache.update, we just need the path for deletion from cache
	deletesOwner := make(sdcpb.Paths, 0, len(deletesOwnerUpdates))
	// so collect the paths
	for _, d := range deletesOwnerUpdates {
		deletesOwner = append(deletesOwner, d.Path())
	}
	return deletesOwner
}

// GetHighesPrecedence return the new cache.Update entried from the tree that are the highes priority.
// If the onlyNewOrUpdated option is set to true, only the New or Updated entries will be returned
// It will append to the given list and provide a new pointer to the slice
func (r *RootEntry) GetHighestPrecedence(onlyNewOrUpdated bool) LeafVariantSlice {
	return r.sharedEntryAttributes.GetHighestPrecedence(make(LeafVariantSlice, 0), onlyNewOrUpdated, false)
}

// GetDeletes returns the paths that due to the Tree content are to be deleted from the southbound device.
func (r *RootEntry) GetDeletes(aggregatePaths bool) (types.DeleteEntriesList, error) {
	deletes := []types.DeleteEntry{}
	return r.sharedEntryAttributes.GetDeletes(deletes, aggregatePaths)
}

func (r *RootEntry) GetAncestorSchema() (*sdcpb.SchemaElem, int) {
	return nil, 0
}

func (r *RootEntry) GetDeviations(ch chan<- *types.DeviationEntry) {
	r.sharedEntryAttributes.GetDeviations(ch, true)
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

// DeleteSubtree Deletes from the tree, all elements of the PathSlice defined branch of the given owner. Return values are remainsToExist and error if an error occured.
func (r *RootEntry) DeleteSubtreePaths(ctx context.Context, deletes types.DeleteEntriesList, intentName string) (bool, error) {
	remainsToExist := true
	for _, del := range deletes {
		remainsToExist, err := r.DeleteSubtree(ctx, del.SdcpbPath(), intentName)
		if err != nil {
			return remainsToExist, err
		}
	}
	return remainsToExist, nil
}
