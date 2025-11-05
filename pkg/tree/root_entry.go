package tree

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	log "github.com/sirupsen/logrus"
)

// RootEntry the root of the cache.Update tree
type RootEntry struct {
	*sharedEntryAttributes
	explicitDeletes *DeletePathSet
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
		explicitDeletes:       NewDeletePaths(),
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
		explicitDeletes:       r.explicitDeletes.DeepCopy(),
	}

	err = tc.SetRoot(result.sharedEntryAttributes)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *RootEntry) RemoveExplicitDeletes(intentName string) *sdcpb.PathSet {
	return r.explicitDeletes.RemoveIntentDeletes(intentName)
}

func (r *RootEntry) AddUpdatesRecursive(ctx context.Context, us types.UpdateSlice, flags *types.UpdateInsertFlags) error {
	var err error
	for idx, u := range us {
		_ = idx
		_, err = r.sharedEntryAttributes.AddUpdateRecursive(ctx, u.Path(), u, flags)
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

	r.explicitDeletes.Add(intentName, intentPrio, importer.GetDeletes())

	return e.ImportConfig(ctx, importer, intentName, intentPrio, flags)
}

func (r *RootEntry) AddExplicitDeletes(intentName string, priority int32, pathset *sdcpb.PathSet) {
	r.explicitDeletes.Add(intentName, priority, pathset)
}

func (r *RootEntry) Validate(ctx context.Context, vCfg *config.Validation) (types.ValidationResults, *types.ValidationStats) {
	// perform validation
	// we use a channel and cumulate all the errors
	validationResultEntryChan := make(chan *types.ValidationResultEntry, 10)
	validationStats := types.NewValidationStats()

	// start validation in a seperate goroutine
	go func() {
		r.sharedEntryAttributes.Validate(ctx, validationResultEntryChan, validationStats, vCfg)
		close(validationResultEntryChan)
	}()

	// create a ValidationResult struct
	validationResult := types.ValidationResults{}

	syncWait := &sync.WaitGroup{}
	syncWait.Add(1)
	go func() {
		// read from the validationResult channel
		for e := range validationResultEntryChan {
			validationResult.AddEntry(e)
		}
		syncWait.Done()
	}()

	syncWait.Wait()
	return validationResult, validationStats
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
	return r.sharedEntryAttributes.GetHighestPrecedence(make(LeafVariantSlice, 0), onlyNewOrUpdated, false, false)
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

func (r *RootEntry) TreeExport(owner string, priority int32, deviation bool) (*tree_persist.Intent, error) {
	treeExport, err := r.sharedEntryAttributes.TreeExport(owner)
	if err != nil {
		return nil, err
	}

	explicitDeletes := r.explicitDeletes.GetByIntentName(owner).ToPathSlice()

	var rootExportEntry *tree_persist.TreeElement
	if len(treeExport) != 0 {
		rootExportEntry = treeExport[0]
	}

	if rootExportEntry != nil || len(explicitDeletes) > 0 {
		return &tree_persist.Intent{
			IntentName:      owner,
			Root:            rootExportEntry,
			Priority:        priority,
			Deviation:       deviation,
			ExplicitDeletes: explicitDeletes,
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
func (r *RootEntry) DeleteBranchPaths(ctx context.Context, deletes types.DeleteEntriesList, intentName string) error {
	for _, del := range deletes {
		err := r.DeleteBranch(ctx, del.SdcpbPath(), intentName)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RootEntry) FinishInsertionPhase(ctx context.Context) error {

	edvs := ExplicitDeleteVisitors{}

	// apply the explicit deletes
	for deletePathPrio := range r.explicitDeletes.Items() {
		edv := NewExplicitDeleteVisitor(deletePathPrio.GetOwner(), deletePathPrio.GetPrio())

		for path := range deletePathPrio.PathItems() {
			// set the priority
			// navigate to the stated path
			entry, err := r.NavigateSdcpbPath(ctx, path)
			if err != nil {
				log.Warnf("Applying explicit delete: path %s not found, skipping", path.ToXPath(false))
			}

			// walk the whole branch adding the explicit delete leafvariant
			err = entry.Walk(ctx, edv)
			if err != nil {
				return err
			}
			edvs[deletePathPrio.GetOwner()] = edv
		}
	}
	log.Debugf("ExplicitDeletes added: %s", utils.MapToString(edvs.Stats(), ", ", func(k string, v int) string {
		return fmt.Sprintf("%s=%d", k, v)
	}))

	return r.sharedEntryAttributes.FinishInsertionPhase(ctx)
}
