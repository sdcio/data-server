package api

import (
	"context"
	"fmt"

	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// EntryFactory is a function type for creating new Entry instances
type EntryFactory func(ctx context.Context, parent Entry, pathElemName string, tc TreeContext) (Entry, error)

var (
	newEntryFunc EntryFactory
)

// RegisterEntryFactory registers the factory function for creating Entry instances
// This is called by the tree package during initialization
func RegisterEntryFactory(factory EntryFactory) {
	if newEntryFunc != nil {
		panic("EntryFactory already registered")
	}
	newEntryFunc = factory
}

// NewEntry creates a new Entry instance as a child of the given parent
// The parent's AddChild method is called to register the new entry
func NewEntry(ctx context.Context, parent Entry, pathElemName string, tc TreeContext) (Entry, error) {
	if newEntryFunc == nil {
		return nil, fmt.Errorf("EntryFactory not registered")
	}
	return newEntryFunc(ctx, parent, pathElemName, tc)
}

// Entry is the primary Element of the Tree.
type Entry interface {
	// PathName returns the last Path element, the name of the Entry
	PathName() string
	// GetLevel returns the depth of the Entry in the tree
	GetLevel() int
	// addChild Add a child entry
	AddChild(context.Context, Entry) error
	// StringIndent debug tree struct as indented string slice
	StringIndent(result []string) []string
	// GetSchema returns the *sdcpb.SchemaElem of the Entry
	GetSchema() *sdcpb.SchemaElem
	// IsRoot returns true if the Entry is the root of the tree
	IsRoot() bool
	// FinishInsertionPhase indicates, that the insertion of Entries into the tree is over
	// Hence calculations for e.g. choice/case can be performed.
	FinishInsertionPhase(ctx context.Context) error
	// GetParent returns the parent entry
	GetParent() Entry
	// SdcpbPath returns the sdcpb.Path struct for the Entry
	SdcpbPath() *sdcpb.Path
	// // // GetSchemaKeys checks for the schema of the entry, and returns the defined keys
	// // GetSchemaKeys() []string
	// // GetRootBasedEntryChain returns all the entries starting from the root down to the actual Entry.
	// GetRootBasedEntryChain() []Entry
	// remainsToExist indicates if a LeafEntry for this entry will survive the update.
	// Since we add running to the tree, there will always be Entries, that will disappear in the
	// as part of the SetIntent process. We need to consider this, when evaluating e.g. LeafRefs.
	// The returned boolean will in indicate if the value remains existing (true) after the setintent.
	// Or will disappear from device (running) as part of the update action.
	RemainsToExist() bool
	// shouldDelete returns true if an explicit delete should be issued for the given branch
	ShouldDelete() bool
	// canDelete checks if the entry can be Deleted.
	// This is e.g. to cover e.g. defaults and running. They can be deleted, but should not, they are basically implicitly existing.
	// In caomparison to
	//    - remainsToExists() returns true, because they remain to exist even though implicitly.
	//    - shouldDelete() returns false, because no explicit delete should be issued for them.
	CanDelete() bool
	GetChildMap() *ChildMap
	GetChilds(types.DescendMethod) EntryMap

	// // DeleteBranch Deletes from the tree, all elements of the PathSlice defined branch of the given owner
	// DeleteBranch(ctx context.Context, path *sdcpb.Path, owner string) (err error)
	// GetListChilds collects all the childs of the list. In the tree we store them seperated into their key branches.
	// this is collecting all the last level key entries.
	// GetListChilds() ([]Entry, error)
	DeepCopy(tc TreeContext, parent Entry) (Entry, error)
	GetLeafVariants() *LeafVariants
	CanDeleteBranch(keepDefault bool) bool
	DeleteCanDeleteChilds(keepDefault bool)
	// GetTreeContext returns the TreeContext of the Entry, which holds global information about the tree
	// and is used for certain operations that require access to this global information.
	GetTreeContext() TreeContext
	// ChoicesResolvers returns the choice case resolvers for the entry, if any
	ChoicesResolvers() ChoiceResolvers
}

type DescendMethod int

const (
	DescendMethodAll DescendMethod = iota
	DescendMethodActiveChilds
)

type HighestPrecedenceFilter func(le *LeafEntry) bool

func HighestPrecedenceFilterAll(le *LeafEntry) bool {
	return true
}
func HighestPrecedenceFilterWithoutNew(le *LeafEntry) bool {
	return !le.IsNew
}
func HighestPrecedenceFilterWithoutDeleted(le *LeafEntry) bool {
	return !le.Delete
}
