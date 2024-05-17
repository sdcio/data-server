package tree

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/sdcio/data-server/pkg/cache"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

const (
	KeysIndexSep = "_"
)

type EntryImpl struct {
	*sharedEntryAttributes
}

type Entry interface {
	// Path returns the Path as []string
	Path() []string
	// PathName returns the last Path element, the name of the Entry
	PathName() string
	// AddChild Add a child entry
	AddChild(context.Context, Entry) error
	// AddCacheUpdateRecursive Add the given cache.Update to the tree
	AddCacheUpdateRecursive(ctx context.Context, u *cache.Update, new bool) error
	// StringIndent debug tree struct as indented string slice
	StringIndent(result []string) []string
	// GetHighesPrio return the new cache.Update entried from the tree that are the highes priority.
	// If the onlyNewOrUpdated option is set to true, only the New or Updated entries will be returned
	// It will append to the given list and provide a new pointer to the slice
	GetHighesPrio(u []*cache.Update, onlyNewOrUpdated bool) []*cache.Update
	// GetByOwner returns the branches Updates by owner
	GetByOwner(owner string, result []*LeafEntry) []*LeafEntry
	// MarkOwnerDelete Sets the delete flag on all the LeafEntries belonging to the given owner.
	MarkOwnerDelete(o string)
	// GetDeletes returns the cache-updates that are not updated, have no lower priority value left and hence should be deleted completely
	GetDeletes([][]string) [][]string
	// Walk takes the EntryVisitor and applies it to every Entry in the tree
	Walk(f EntryVisitor) error
	// ShouldDelete indicated if there is no LeafEntry left and the Entry is to be deleted
	ShouldDelete() bool
	// IsDeleteKeyAttributesInLevelDown TODO
	IsDeleteKeyAttributesInLevelDown(level int, names []string) (bool, [][]string)
	ValidateMandatory() error
	GetLowestPriorityValueOfBranch() int32
}

func newEntry(ctx context.Context, parent Entry, pathElemName string, tc *TreeContext) (*EntryImpl, error) {

	sea, err := newSharedEntryAttributes(ctx, parent, pathElemName, tc)
	if err != nil {
		return nil, err
	}

	newEntry := &EntryImpl{
		sharedEntryAttributes: sea,
	}
	err = parent.AddChild(ctx, newEntry)
	return newEntry, err
}

type LeafEntry struct {
	*cache.Update
	IsNew     bool
	Delete    bool
	IsUpdated bool
}

// MarkUpdate indicate that the entry is an Updated value
func (l *LeafEntry) MarkUpdate(u *cache.Update) {
	// set the new value
	l.Update = u
	// set the update flag
	l.IsUpdated = true
	// reset the delete flag
	l.Delete = false
}

// MarkDelete indicate that the entry is to be deleted
func (l *LeafEntry) MarkDelete() {
	l.Delete = true
}

func (l *LeafEntry) String() string {
	tv, err := l.Value()
	var v string
	if err != nil {
		v = err.Error()
	} else {
		v = tv.String()
	}
	return fmt.Sprintf("Owner: %s, Priority: %d, Value: %s, New: %t, Delete: %t, Update: %t", l.Owner(), l.Priority(), v, l.IsNew, l.Delete, l.IsUpdated)
}

func NewLeafEntry(c *cache.Update, new bool) *LeafEntry {
	return &LeafEntry{
		Update: c,
		IsNew:  new,
	}
}

// sharedEntryAttributes contains the attributes shared by Entry and RootEntry
type sharedEntryAttributes struct {
	// parent entry, nil for the root Entry
	parent Entry
	// pathElemName the path elements name the entry represents
	pathElemName string
	// childs mutual exclusive with LeafVariants
	childs map[string]Entry
	// leafVariants mutual exclusive with Childs
	// If Entry is a leaf it can hold multiple leafVariants
	leafVariants LeafVariants
	// schema the schema element for this entry
	schema *sdcpb.SchemaElem

	choicesResolvers choiceCasesResolvers

	treeContext *TreeContext
}

func newSharedEntryAttributes(ctx context.Context, parent Entry, pathElemName string, tc *TreeContext) (*sharedEntryAttributes, error) {

	s := &sharedEntryAttributes{
		parent:       parent,
		pathElemName: pathElemName,
		childs:       map[string]Entry{},
		leafVariants: newLeafVariants(),
		treeContext:  tc,
	}

	// TODO: performance - need to go func
	schemaResp, err := tc.treeSchemaCacheClient.GetSchema(ctx, s.Path())
	if err != nil {
		return nil, err
	}

	s.schema = schemaResp.GetSchema()

	s.initChoiceCasesResolvers()

	return s, nil
}

// func (s *sharedEntryAttributes) fetchDependencies(ctx context.Context) error {
// 	// if it is not a schema element (key value), return
// 	if s.schema == nil {
// 		return nil
// 	}

// 	// TODO add other dependencies (mandatory, must, leafref etc.)
// 	return nil
// }

func (s *sharedEntryAttributes) fetchDependenciesChoice(ctx context.Context, elem string) error {
	if len(s.choicesResolvers) == 0 {
		return nil
	}

	paths := [][]string{}

	for _, c := range s.choicesResolvers.GetChoiceElementNeighbors(elem) {
		// add the sub-elements name to the parent path.
		path := append(s.Path(), c)
		// add the resulting path to the collected paths
		paths = append(paths, path)
	}
	// query the cache for the data
	updates := s.treeContext.treeSchemaCacheClient.ReadIntended(ctx, &cache.Opts{}, paths, 0)

	// iterate through the received updates and add them.
	for _, u := range updates {
		err := s.AddCacheUpdateRecursive(ctx, u, false)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *sharedEntryAttributes) GetLevel() int {
	return len(s.Path())
}

// Walk takes the EntryVisitor and applies it to every Entry in the tree
func (s *sharedEntryAttributes) Walk(f EntryVisitor) error {

	// TODO: COME UP WITH SOME CLEVER CONCURRENCY

	// execute the function locally
	err := f(s)
	if err != nil {
		return err
	}

	// trigger the execution on all childs
	for _, c := range s.childs {
		err := c.Walk(f)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *sharedEntryAttributes) IsDeleteKeyAttributesInLevelDown(level int, names []string) (bool, [][]string) {
	if level > 0 {
		for _, v := range s.childs {
			return v.IsDeleteKeyAttributesInLevelDown(level-1, names)
		}
	}
	// if we're at the right level, check the keys for deletion
	for _, n := range names {
		c, exists := s.childs[n]
		// these keys should aways exist, so for now we do not catch the non existing key case
		if exists && !c.ShouldDelete() {
			return false, nil
		}
	}
	return true, [][]string{s.Path()}
}

// ShouldDelete flag if the leafvariant or entire branch is marked for deletion
func (s *sharedEntryAttributes) ShouldDelete() bool {
	// if leafeVariants exist, delegate the call to the leafVariants
	if len(s.leafVariants) > 0 {
		return s.leafVariants.ShouldDelete()
	}
	// otherwise query the childs
	for _, c := range s.childs {
		// if a single child exists that should not be deleted, exit early with a false
		if !c.ShouldDelete() {
			return false
		}
	}
	// otherwise all childs reported true, and we can report true as well
	return true
}

func (s *sharedEntryAttributes) GetDeletes(deletes [][]string) [][]string {

	// if entry is a container type, check the keys, to be able to
	// issue a delte for the whole branch at once via keys
	switch sch := s.schema.GetSchema().(type) {
	case *sdcpb.SchemaElem_Container:
		keySchemas := sch.Container.Keys

		if len(keySchemas) > 0 {
			var keys []string
			for _, k := range keySchemas {
				keys = append(keys, k.Name)
			}

			for _, c := range s.childs {
				if doDelete, paths := c.IsDeleteKeyAttributesInLevelDown(len(keys)-1, keys); doDelete {
					deletes = append(deletes, paths...)
				} else {
					deletes = c.GetDeletes(deletes)
				}
			}
			return deletes
		}
	}

	if s.leafVariants.ShouldDelete() {
		return append(deletes, s.leafVariants[0].GetPath())
	}

	for _, e := range s.childs {
		deletes = e.GetDeletes(deletes)
	}
	return deletes
}

func (s *sharedEntryAttributes) GetByOwner(owner string, result []*LeafEntry) []*LeafEntry {
	lv := s.leafVariants.GetByOwner(owner)
	if lv != nil {
		result = append(result, lv)
	}

	// continue with childs
	for _, c := range s.childs {
		result = c.GetByOwner(owner, result)
	}
	return result
}

func (s *sharedEntryAttributes) GetLowestPriorityValueOfBranch() int32 {
	result := int32(math.MaxInt32)
	for _, e := range s.childs {
		if val := e.GetLowestPriorityValueOfBranch(); val < result {
			result = val
		}
	}
	if val := s.leafVariants.GetLowestPriorityValue(); val < result {
		result = val
	}

	return result
}

func (s *sharedEntryAttributes) Path() []string {
	// special handling for root node
	if s.parent == nil {
		return []string{}
	}
	return append(s.parent.Path(), s.pathElemName)
}

func (s *sharedEntryAttributes) PathName() string {
	return s.pathElemName
}

func (s *sharedEntryAttributes) String() string {
	return strings.Join(s.parent.Path(), "/")
}

func (s *sharedEntryAttributes) AddChild(ctx context.Context, e Entry) error {
	// make sure Entry should not only hold LeafEntries
	if len(s.leafVariants) > 0 {
		// An exception are presence containers
		contSchema, is_container := s.schema.Schema.(*sdcpb.SchemaElem_Container)
		if !is_container && !contSchema.Container.IsPresence {
			return fmt.Errorf("cannot add child to %s since it holds Leafs", s)
		}
	}
	// check the path of child is a subpath of s
	if !slices.Equal(s.Path(), e.Path()[:len(e.Path())-1]) {
		return fmt.Errorf("adding Child with diverging path, parent: %s, child: %s", s, strings.Join(e.Path()[:len(e.Path())-1], "/"))
	}
	s.childs[e.PathName()] = e

	// the child might be part of a choice/case, lets check that and populate the
	// tree with all the items that belong to the same choice, to then be able to
	// make priority based decisions for one or the other case solely within the tree.
	err := s.fetchDependenciesChoice(ctx, e.PathName())
	if err != nil {
		return err
	}

	return nil
}

func (s *sharedEntryAttributes) GetHighesPrio(result []*cache.Update, onlyNewOrUpdated bool) []*cache.Update {
	// try to get get the highes prio LeafVariant make sure it exists (!= nil)
	// and add it to the result if it is NEW
	lv := s.leafVariants.GetHighesPrio(onlyNewOrUpdated)
	if lv != nil {
		result = append(result, lv.Update)
	}

	// continue with childs
	for _, c := range s.childs {
		result = c.GetHighesPrio(result, onlyNewOrUpdated)
	}
	return result
}

func (s *sharedEntryAttributes) ValidateMandatory() error {
	if s.schema != nil {
		switch s.schema.GetSchema().(type) {
		case *sdcpb.SchemaElem_Container:
			for _, c := range s.schema.GetContainer().MandatoryChildren {
				// first check if the mandatory value is set via the intent, e.g. part of the tree already
				v, existsInTree := s.childs[c]

				// if not the path exists in the tree and is not to be deleted, then lookup in the paths index of the store
				// and see if such path exists, if not raise the error
				if !(existsInTree && !v.ShouldDelete()) {
					if !s.treeContext.PathExists(append(s.Path(), c)) {
						return fmt.Errorf("%s: mandatory child %s does not exist", s.Path(), c)
					}
				}
			}
		}
	}
	// continue with childs
	for _, c := range s.childs {
		err := c.ValidateMandatory()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *sharedEntryAttributes) initChoiceCasesResolvers() {
	if s.schema == nil {
		return
	}

	// extract container schema
	var ci *sdcpb.ChoiceInfo
	switch s.schema.GetSchema().(type) {
	case *sdcpb.SchemaElem_Container:
		ci = s.schema.GetContainer().GetChoiceInfo()
	}

	// create a new choiceCasesResolvers struct
	choicesResolvers := choiceCasesResolvers{}

	// iterate through choices defined in schema
	for choiceName, choice := range ci.GetChoice() {
		// add the choice to the choiceCasesResolvers
		actualResolver := choicesResolvers.AddChoice(choiceName)
		// iterate through cases
		for caseName, choiceCase := range choice.GetCase() {
			// add cases and their depending elements / attributes to the case
			actualResolver.AddCase(caseName, choiceCase.GetElements())
		}
	}
	// set the resolver in the sharedEntryAttributes
	s.choicesResolvers = choicesResolvers
}

func (s *sharedEntryAttributes) filterActiveChoiceCaseChilds() map[string]Entry {
	if s.schema == nil {
		return s.childs
	}
	// if choice/cases exist, process it
	for _, choiceResolver := range s.choicesResolvers {
		for _, elem := range choiceResolver.GetElementNames() {
			child, childExists := s.childs[elem]
			if childExists {
				v := child.GetLowestPriorityValueOfBranch()
				if v < math.MaxInt32 {
					choiceResolver.SetValue(elem, v)
				}
			}
			// Query the Index, stored in the treeContext for the per branch highes precedence
			if val := s.treeContext.GetBranchesLowestPriorityValue(s.Path(), CacheUpdateFilterExcludeOwner(s.treeContext.GetActualOwner())); val < math.MaxInt32 {
				choiceResolver.SetValue(elem, val)
			}
		}
	}

	skipAttributesList := s.choicesResolvers.GetSkipElements()
	result := map[string]Entry{}
	// optimization option: sort the slices and forward in parallel, lifts extra burden that the contains call holds.
	for childName, child := range s.childs {
		if slices.Contains(skipAttributesList, childName) {
			continue
		}
		result[childName] = child
	}
	return result
}

// func (s *sharedEntryAttributes) ValidateChoices() error {
// 	if s.schema != nil {
// 		switch s.schema.GetSchema().(type) {
// 		case *sdcpb.SchemaElem_Container:
// 			choiceCasePrio := map[string]map[string]int32{}
// 			for choiceName, choice := range s.schema.GetContainer().GetChoiceInfo().GetChoice() {
// 				for caseName, choiceCase := range choice.GetCase() {
// 					for _, elem := range choiceCase.GetElements() {
// 						prioValue := int32(math.MaxInt32)
// 						child, childExists := s.childs[elem]
// 						if childExists {
// 							prioValue = child.GetLowestPriorityValueOfBranch()
// 						}
// 						if storeValue := s.treeContext.GetBranchesLowestPriorityValue(append(s.Path(), elem)); storeValue < prioValue {
// 							prioValue = storeValue
// 						}
// 						choiceCasePrio[choiceName][caseName] = prioValue
// 					}
// 				}
// 			}
// 		}
// 	}

// 	// continue with childs
// 	for _, c := range s.childs {
// 		err := c.ValidateChoices()
// 		if err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

// StringIndent returns the sharedEntryAttributes in its string representation
// The string is intented according to the nesting level in the yang model
func (s *sharedEntryAttributes) StringIndent(result []string) []string {
	result = append(result, strings.Repeat("  ", s.GetLevel())+s.pathElemName)

	// ranging over children and LeafVariants
	// then should be mutual exclusive, either a node has children or LeafVariants

	// range over children
	for _, c := range s.childs {
		result = c.StringIndent(result)
	}
	// range over LeafVariants
	for _, l := range s.leafVariants {
		result = append(result, fmt.Sprintf("%s -> %s", strings.Repeat("  ", s.GetLevel()), l.String()))
	}
	return result
}

// MarkOwnerDelete Sets the delete flag on all the LeafEntries belonging to the given owner.
func (s *sharedEntryAttributes) MarkOwnerDelete(o string) {
	lvEntry := s.leafVariants.GetByOwner(o)
	if lvEntry != nil {
		lvEntry.MarkDelete()
	}
	for _, child := range s.childs {
		child.MarkOwnerDelete(o)
	}
}

func (r *sharedEntryAttributes) AddCacheUpdateRecursive(ctx context.Context, c *cache.Update, new bool) error {
	idx := 0
	// if it is the root node, index remains == 0
	if r.parent != nil {
		idx = r.GetLevel()
	}
	// end of path reached, add LeafEntry
	// continue with recursive add otherwise
	if idx == len(c.GetPath()) {
		// Check if LeafEntry with given owner already exists
		if leafVariant := r.leafVariants.GetByOwner(c.Owner()); leafVariant != nil {
			if leafVariant.EqualSkipPath(c) {
				// it seems like the element was not deleted, so drop the delete flag
				leafVariant.Delete = false
			} else {
				// if a leafentry of the same owner exists with different value, mark it for update
				leafVariant.MarkUpdate(c)
			}
		} else {
			// if LeafVaraint with same owner does not exist, add the new entry
			r.leafVariants = append(r.leafVariants, NewLeafEntry(c, new))
		}
		return nil
	}

	var e Entry
	var err error
	var exists bool
	// if child does not exist, create Entry
	if e, exists = r.childs[c.GetPath()[idx]]; !exists {
		e, err = newEntry(ctx, r, c.GetPath()[idx], r.treeContext)
		if err != nil {
			return err
		}
	}
	return e.AddCacheUpdateRecursive(ctx, c, new)
}

type UpdateSlice []*cache.Update

// GetHighesPriorityValue returns the highes priority value of all the containing Updates
func (u UpdateSlice) GetLowestPriorityValue(filters []CacheUpdateFilter) int32 {
	result := int32(math.MaxInt32)
	for _, entry := range u {
		if entry.Priority() < result && ApplyCacheUpdateFilters(entry, filters) {
			result = entry.Priority()
		}
	}
	return result
}

// RootEntry the root of the cache.Update tree
type RootEntry struct {
	*sharedEntryAttributes
}

func NewTreeRoot(ctx context.Context, tc *TreeContext) (*RootEntry, error) {
	sea, err := newSharedEntryAttributes(ctx, nil, "", tc)
	if err != nil {
		return nil, err
	}

	return &RootEntry{
		sharedEntryAttributes: sea,
	}, nil
}

func (r *RootEntry) String() string {
	s := []string{}
	s = r.sharedEntryAttributes.StringIndent(s)
	return strings.Join(s, "\n")
}

// GetUpdatesForOwner returns the updates that have been calculated for the given intent / owner
func (r *RootEntry) GetUpdatesForOwner(owner string) []*cache.Update {
	// retrieve all the entries from the tree that belong to the given
	// Owner / Intent, skipping the once marked for deletion
	// this is to insert / update entries in the cache.
	return LeafEntriesToCacheUpdates(r.getByOwnerFiltered(owner, FilterNonDeleted))
}

// GetDeletesForOwner returns the deletes that have been calculated for the given intent / owner
func (r *RootEntry) GetDeletesForOwner(owner string) [][]string {
	// retrieve all entries from the tree that belong to the given user
	// and that are marked for deletion.
	// This is to cover all the cases where an intent was changed and certain
	// part of the config got deleted.
	deletesOwnerUpdates := LeafEntriesToCacheUpdates(r.getByOwnerFiltered(owner, FilterDeleted))
	// they are retrieved as cache.update, we just need the path for deletion from cache
	deletesOwner := make([][]string, 0, len(deletesOwnerUpdates))
	// so collect the paths
	for _, d := range deletesOwnerUpdates {
		deletesOwner = append(deletesOwner, d.GetPath())
	}
	return deletesOwner
}

// GetHighesPrio return the new cache.Update entried from the tree that are the highes priority.
// If the onlyNewOrUpdated option is set to true, only the New or Updated entries will be returned
// It will append to the given list and provide a new pointer to the slice
func (r *RootEntry) GetHighesPrio(onlyNewOrUpdated bool) []*cache.Update {
	return r.sharedEntryAttributes.GetHighesPrio(make([]*cache.Update, 0), onlyNewOrUpdated)
}

func (r *RootEntry) GetDeletes() [][]string {
	deletes := [][]string{}
	return r.sharedEntryAttributes.GetDeletes(deletes)
}

func (r *RootEntry) GetTreeContext() *TreeContext {
	return r.treeContext
}

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

type EntryVisitor func(s *sharedEntryAttributes) error

// // TreeWalkerSchemaRetriever returns an EntryVisitor, that populates the tree entries with the corresponding schema entries.
// func TreeWalkerSchemaRetriever(ctx context.Context, scb SchemaClient.SchemaClientBound) EntryVisitor {
// 	// the schemaIndex is used as a lookup cache for Schema elements,
// 	// to prevent repetetive requests for the same schema element
// 	schemaIndex := map[string]*sdcpb.SchemaElem{}

// 	return func(s *sharedEntryAttributes) error {
// 		// if schema is already set, return early
// 		if s.schema != nil {
// 			return nil
// 		}

// 		// convert the []string path into sdcpb.path for schema retrieval
// 		sdcpbPath, err := scb.ToPath(ctx, s.Path())
// 		if err != nil {
// 			return err
// 		}

// 		// // check if the actual path points to a key value (the last path element contains a key)
// 		// // if so, we can skip querying the schema server
// 		// if len(sdcpbPath.Elem) > 0 && len(sdcpbPath.Elem[len(sdcpbPath.Elem)-1].Key) > 0 {
// 		// 	// s.schema remains nil
// 		// 	// s.isSchemaElement remains false
// 		// 	return nil
// 		// }

// 		// convert the path into a keyless path, for schema index lookups.
// 		keylessPathSlice := utils.ToStrings(sdcpbPath, false, true)
// 		keylessPath := strings.Join(keylessPathSlice, KeysIndexSep)

// 		// lookup schema in schemaindex, preventing consecutive gets from the schema server
// 		if v, exists := schemaIndex[keylessPath]; exists {
// 			// set the schema retrieved from SchemaIndex
// 			s.schema = v
// 			// we're done, schema is set, return
// 			return nil
// 		}

// 		// if schema wasn't found in index, go and fetch it
// 		schemaRsp, err := scb.GetSchema(ctx, sdcpbPath)
// 		if err != nil {
// 			return err
// 		}

// 		// store schema in schemaindex for next lookup
// 		schemaIndex[keylessPath] = schemaRsp.GetSchema()
// 		// set the sharedEntryAttributes related values
// 		s.schema = schemaRsp.GetSchema()
// 		return nil
// 	}
// }
