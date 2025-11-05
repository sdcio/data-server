package tree

import (
	"context"
	"math"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/sdc-protos/tree_persist"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

const (
	KeysIndexSep       = "_"
	DefaultValuesPrio  = int32(math.MaxInt32 - 90)
	DefaultsIntentName = "default"
	RunningValuesPrio  = int32(math.MaxInt32 - 100)
	RunningIntentName  = "running"
	ReplaceValuesPrio  = int32(math.MaxInt32 - 110)
	ReplaceIntentName  = "replace"
)

// newEntry constructor for Entries
func newEntry(ctx context.Context, parent Entry, pathElemName string, tc *TreeContext) (*sharedEntryAttributes, error) {
	// create a new sharedEntryAttributes instance
	sea, err := newSharedEntryAttributes(ctx, parent, pathElemName, tc)
	if err != nil {
		return nil, err
	}

	// add the Entry as a child to the parent Entry
	err = parent.addChild(ctx, sea)
	return sea, err
}

// Entry is the primary Element of the Tree.
type Entry interface {
	// PathName returns the last Path element, the name of the Entry
	PathName() string
	// GetLevel returns the depth of the Entry in the tree
	GetLevel() int
	// addChild Add a child entry
	addChild(context.Context, Entry) error
	// getOrCreateChilds retrieves the sub-child pointed at by the path.
	// if the path does not exist in its full extend, the entries will be added along the way
	// if the path does not point to a schema defined path an error will be raise
	getOrCreateChilds(ctx context.Context, path *sdcpb.Path) (Entry, error)
	// AddUpdateRecursive Add the given cache.Update to the tree
	AddUpdateRecursive(ctx context.Context, relativePath *sdcpb.Path, u *types.Update, flags *types.UpdateInsertFlags) (Entry, error)
	// StringIndent debug tree struct as indented string slice
	StringIndent(result []string) []string
	// GetHighesPrio return the new cache.Update entried from the tree that are the highes priority.
	// If the onlyNewOrUpdated option is set to true, only the New or Updated entries will be returned
	// It will append to the given list and provide a new pointer to the slice
	GetHighestPrecedence(result LeafVariantSlice, onlyNewOrUpdated bool, includeDefaults bool, includeExplicitDelete bool) LeafVariantSlice
	// getHighestPrecedenceLeafValue returns the highest LeafValue of the Entry at hand
	// will return an error if the Entry is not a Leaf
	getHighestPrecedenceLeafValue(context.Context) (*LeafEntry, error)
	// GetByOwner returns the branches Updates by owner
	GetByOwner(owner string, result []*LeafEntry) LeafVariantSlice
	// // markOwnerDelete Sets the delete flag on all the LeafEntries belonging to the given owner.
	// MarkOwnerDelete(o string, onlyIntended bool)
	// GetDeletes returns the cache-updates that are not updated, have no lower priority value left and hence should be deleted completely
	GetDeletes(entries []types.DeleteEntry, aggregatePaths bool) ([]types.DeleteEntry, error)
	// Walk takes the EntryVisitor and applies it to every Entry in the tree
	Walk(ctx context.Context, v EntryVisitor) error
	// Validate kicks off validation
	ValidateLevel(ctx context.Context, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats, vCfg *config.Validation)
	// validateMandatory the Mandatory schema field
	validateMandatory(ctx context.Context, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats)
	// validateMandatoryWithKeys is an internally used function that us called by validateMandatory in case
	// the container has keys defined that need to be skipped before the mandatory attributes can be checked
	validateMandatoryWithKeys(ctx context.Context, level int, attributes []string, choiceName string, resultChan chan<- *types.ValidationResultEntry)
	// getHighestPrecedenceValueOfBranch returns the highes Precedence Value (lowest Priority value) of the brach that starts at this Entry
	getHighestPrecedenceValueOfBranch(filter HighestPrecedenceFilter) int32
	// GetSchema returns the *sdcpb.SchemaElem of the Entry
	GetSchema() *sdcpb.SchemaElem
	// IsRoot returns true if the Entry is the root of the tree
	IsRoot() bool
	// FinishInsertionPhase indicates, that the insertion of Entries into the tree is over
	// Hence calculations for e.g. choice/case can be performed.
	FinishInsertionPhase(ctx context.Context) error
	// GetParent returns the parent entry
	GetParent() Entry
	NavigateSdcpbPath(ctx context.Context, path *sdcpb.Path) (Entry, error)
	// NavigateLeafRef follows the leafref and returns the referenced entry
	NavigateLeafRef(ctx context.Context) ([]Entry, error)
	// GetFirstAncestorWithSchema returns the first parent node which has a schema set.
	// if the parent has no schema (is a key element in the tree) it will recurs the call to the parents parent.
	// the level of recursion is indicated via the levelUp attribute
	GetFirstAncestorWithSchema() (ancestor Entry, levelUp int)
	// SdcpbPath returns the sdcpb.Path struct for the Entry
	SdcpbPath() *sdcpb.Path
	// GetSchemaKeys checks for the schema of the entry, and returns the defined keys
	GetSchemaKeys() []string
	// GetRootBasedEntryChain returns all the entries starting from the root down to the actual Entry.
	GetRootBasedEntryChain() []Entry
	// GetRoot returns the Trees Root Entry
	GetRoot() Entry
	// remainsToExist indicates if a LeafEntry for this entry will survive the update.
	// Since we add running to the tree, there will always be Entries, that will disappear in the
	// as part of the SetIntent process. We need to consider this, when evaluating e.g. LeafRefs.
	// The returned boolean will in indicate if the value remains existing (true) after the setintent.
	// Or will disappear from device (running) as part of the update action.
	remainsToExist() bool
	// shouldDelete returns true if an explicit delete should be issued for the given branch
	shouldDelete() bool
	// canDelete checks if the entry can be Deleted.
	// This is e.g. to cover e.g. defaults and running. They can be deleted, but should not, they are basically implicitly existing.
	// In caomparison to
	//    - remainsToExists() returns true, because they remain to exist even though implicitly.
	//    - shouldDelete() returns false, because no explicit delete should be issued for them.
	canDelete() bool
	GetChilds(DescendMethod) EntryMap
	GetChild(name string) (Entry, bool) // entry, exists
	FilterChilds(keys map[string]string) ([]Entry, error)
	// ToJson returns the Tree contained structure as JSON
	// use e.g. json.MarshalIndent() on the returned struct
	ToJson(onlyNewOrUpdated bool) (any, error)
	// ToJsonIETF returns the Tree contained structure as JSON_IETF
	// use e.g. json.MarshalIndent() on the returned struct
	ToJsonIETF(onlyNewOrUpdated bool) (any, error)
	// toJsonInternal the internal function that produces JSON and JSON_IETF
	// Not for external usage
	toJsonInternal(onlyNewOrUpdated bool, ietf bool) (j any, err error)
	// ToXML returns the tree and its current state in the XML representation used by netconf
	ToXML(onlyNewOrUpdated bool, honorNamespace bool, operationWithNamespace bool, useOperationRemove bool) (*etree.Document, error)
	toXmlInternal(parent *etree.Element, onlyNewOrUpdated bool, honorNamespace bool, operationWithNamespace bool, useOperationRemove bool) (doAdd bool, err error)
	// ImportConfig allows importing config data received from e.g. the device in different formats (json, xml) to be imported into the tree.
	ImportConfig(ctx context.Context, importer importer.ImportConfigAdapterElement, intentName string, intentPrio int32, flags *types.UpdateInsertFlags) error
	TreeExport(owner string) ([]*tree_persist.TreeElement, error)
	// DeleteBranch Deletes from the tree, all elements of the PathSlice defined branch of the given owner
	DeleteBranch(ctx context.Context, path *sdcpb.Path, owner string) (err error)
	GetDeviations(ch chan<- *types.DeviationEntry, activeCase bool)
	// GetListChilds collects all the childs of the list. In the tree we store them seperated into their key branches.
	// this is collecting all the last level key entries.
	GetListChilds() ([]Entry, error)
	BreadthSearch(ctx context.Context, path *sdcpb.Path) ([]Entry, error)
	DeepCopy(tc *TreeContext, parent Entry) (Entry, error)
	GetLeafVariantEntries() LeafVariantEntries

	// returns true if the Entry contains leafvariants (presence container, field or leaflist)
	HoldsLeafvariants() bool
	canDeleteBranch(keepDefault bool) bool
	deleteCanDeleteChilds(keepDefault bool)
}

type EntryVisitor interface {
	DescendMethod() DescendMethod
	Visit(ctx context.Context, e Entry) error
	Up()
}

type LeafVariantEntry interface {
	MarkDelete(onlyIntended bool) *LeafEntry
	GetEntry() Entry
	String() string
}

type LeafVariantEntries interface {
	MarkOwnerForDeletion(owner string, onlyIntended bool) *LeafEntry
	GetHighestPrecedence(onlyNewOrUpdated bool, includeDefaults bool, includeExplicitDeletes bool) *LeafEntry
	GetRunning() *LeafEntry
	DeleteByOwner(owner string) *LeafEntry
	AddExplicitDeleteEntry(owner string, priority int32) *LeafEntry
	GetByOwner(owner string) *LeafEntry
	Add(l *LeafEntry)
}

type DescendMethod int

const (
	DescendMethodAll DescendMethod = iota
	DescendMethodActiveChilds
)
