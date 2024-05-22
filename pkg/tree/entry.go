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

// newEntry constructor for Entries
func newEntry(ctx context.Context, parent Entry, pathElemName string, tc *TreeContext) (*EntryImpl, error) {
	// create a new sharedEntryAttributes instance
	sea, err := newSharedEntryAttributes(ctx, parent, pathElemName, tc)
	if err != nil {
		return nil, err
	}

	newEntry := &EntryImpl{
		sharedEntryAttributes: sea,
	}
	// add the Entry as a child to the parent Entry
	err = parent.AddChild(ctx, newEntry)
	return newEntry, err
}

// Entry is the primary Element of the Tree.
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
	GetHighestPrecedence(u UpdateSlice, onlyNewOrUpdated bool) UpdateSlice
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
	// Validate the Mandatory schema field
	ValidateMandatory() error
	// GetHighestPrecedenceValueOfBranch returns the highes Precedence Value (lowest Priority value) of the brach that starts at this Entry
	GetHighestPrecedenceValueOfBranch() int32
	// GetSchema returns the *sdcpb.SchemaElem of the Entry
	GetSchema() *sdcpb.SchemaElem
	// IsRoot returns true if the Entry is the root of the tree
	IsRoot() bool
	// FinishInsertionPhase indicates, that the insertion of Entries into the tree is over
	// Hence calculations for e.g. choice/case can be performed.
	FinishInsertionPhase()
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

	getSchema := true

	// on the root element we cannot query the parent schema.
	// hence skip this part if IsRoot
	if !s.IsRoot() {
		switch schem := parent.GetSchema().GetSchema().(type) {
		case *sdcpb.SchemaElem_Container:
			getSchema = false
			if slices.Contains(schem.Container.Children, pathElemName) {
				getSchema = true
			}

			fields := []string{}
			for _, y := range schem.Container.Fields {
				fields = append(fields, y.Name)
			}
			if slices.Contains(fields, pathElemName) {
				getSchema = true
			}

			fields = []string{}
			for _, y := range schem.Container.Leaflists {
				fields = append(fields, y.Name)
			}
			if slices.Contains(fields, pathElemName) {
				getSchema = true
			}
		}
	}

	if getSchema {
		schemaResp, err := tc.treeSchemaCacheClient.GetSchema(ctx, s.Path())
		if err != nil {
			return nil, err
		}
		s.schema = schemaResp.GetSchema()
	}
	// initialize the choice case resolvers with the schema information
	s.initChoiceCasesResolvers()

	return s, nil
}

// GetSchema return the schema fiels of the Entry
func (s *sharedEntryAttributes) GetSchema() *sdcpb.SchemaElem {
	return s.schema
}

// IsRoot returns true if the element has no parent elements, hence is the root of the tree
func (s *sharedEntryAttributes) IsRoot() bool {
	return s.parent == nil
}

// GetLevel returns the level / depth position of this element in the tree
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

// IsDeleteKeyAttributesInLevelDown On a container that has keys, this function is there to check if the keys
// are being deleted, such that we do not have to delete all entries and attributes, but issue a delete for the path with the specifc keys
// and therby delete the whole branch.
func (s *sharedEntryAttributes) IsDeleteKeyAttributesInLevelDown(level int, names []string) (bool, [][]string) {
	// in the tree, we have Entries with schemas but also the keys as levels in the tree.
	// so level is used to skip the key elements in the tree. Only when level is down to zero, we have the
	// final container at hand.
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

// GetDeletes calculate the deletes that need to be send to the device.
func (s *sharedEntryAttributes) GetDeletes(deletes [][]string) [][]string {
	// if entry is a container type, check the keys, to be able to
	// issue a delte for the whole branch at once via keys
	switch sch := s.schema.GetSchema().(type) {
	case *sdcpb.SchemaElem_Container:

		// deletes for child elements (choice cases) that newly became inactive.
		for _, v := range s.choicesResolvers {
			oldBestCase := v.getOldBestCaseName()
			newBestCase := v.getBestCaseName()
			// so if we have an old and a new best cases (not "") and the names are different,
			// all the old to the deletion list
			if oldBestCase != "" && newBestCase != "" && oldBestCase != newBestCase {
				deletes = append(deletes, append(s.Path(), oldBestCase))
			}
		}

		// now we check if all the keys of the container are to be deleted,
		// if so, simply delete the key path and we do not need to step futher down into the branch.
		keySchemas := sch.Container.Keys
		if len(keySchemas) > 0 {
			var keys []string
			for _, k := range keySchemas {
				keys = append(keys, k.Name)
			}

			for _, c := range s.filterActiveChoiceCaseChilds() {
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

	for _, e := range s.filterActiveChoiceCaseChilds() {
		deletes = e.GetDeletes(deletes)
	}
	return deletes
}

// GetByOwner returns all the LeafEntries that belong to a certain owner.
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

// Path returns the root based path of the Entry
func (s *sharedEntryAttributes) Path() []string {
	// special handling for root node
	if s.parent == nil {
		return []string{}
	}
	return append(s.parent.Path(), s.pathElemName)
}

// PathName returns the name of the Entry
func (s *sharedEntryAttributes) PathName() string {
	return s.pathElemName
}

// String returns a string representation of the Entry
func (s *sharedEntryAttributes) String() string {
	return strings.Join(s.parent.Path(), "/")
}

// AddChild add an entry to the list of child entries for the entry.
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

	return nil
}

// GetHighestPrecedence goes through the whole branch and returns the new and updated cache.Updates.
// These are the updated that will be send to the device.
func (s *sharedEntryAttributes) GetHighestPrecedence(result UpdateSlice, onlyNewOrUpdated bool) UpdateSlice {
	// get the highes precedence LeafeVariant and add it to the list
	lv := s.leafVariants.GetHighestPrecedence(onlyNewOrUpdated)
	if lv != nil {
		result = append(result, lv.Update)
	}

	// continue with childs. Childs are part of choices, process only the "active" (highes precedence) childs
	for _, c := range s.filterActiveChoiceCaseChilds() {
		result = c.GetHighestPrecedence(result, onlyNewOrUpdated)
	}
	return result
}

// GetHighestPrecedenceValueOfBranch goes through all the child branches to find the highes
// precedence value (lowest priority value) for the entire branch and returns it.
func (s *sharedEntryAttributes) GetHighestPrecedenceValueOfBranch() int32 {
	result := int32(math.MaxInt32)
	for _, e := range s.childs {
		if val := e.GetHighestPrecedenceValueOfBranch(); val < result {
			result = val
		}
	}
	if val := s.leafVariants.GetHighestPrecedenceValue(); val < result {
		result = val
	}

	return result
}

// ValidateMandatory validates that all the mandatory attributes,
// defined by the schema are present either in the tree or in the index.
func (s *sharedEntryAttributes) ValidateMandatory() error {
	if s.schema != nil {
		switch s.schema.GetSchema().(type) {
		case *sdcpb.SchemaElem_Container:
			for _, c := range s.schema.GetContainer().MandatoryChildren {
				// first check if the mandatory value is set via the intent, e.g. part of the tree already
				v, existsInTree := s.filterActiveChoiceCaseChilds()[c]

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

// initChoiceCasesResolvers Choices and their cases are defined in the schema.
// We need the information on which choices exist and what the below cases are.
// Therefore the choiceCasesResolvers are initialized with the information.
// At a later stage, when the insertion of values into the tree is completed,
// the choiceCasesResolvers will get the priority values per branch and use these to
// calculate the active case.
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

// FinishInsertionPhase certain values that are costly to calculate but used multiple times
// will be calculated and stored for later use. However therefore the insertion phase into the
// tree needs to be over. Calling this function indicated the end of the phase and thereby triggers the calculation
func (s *sharedEntryAttributes) FinishInsertionPhase() {

	// populate the ChoiceCaseResolvers to determine the active case
	s.populateChoiceCaseResolvers()

	// recurse the call to all (active) entries within the tree.
	// Thereby already using the choiceCaseResolver via filterActiveChoiceCaseChilds()
	for _, child := range s.filterActiveChoiceCaseChilds() {
		child.FinishInsertionPhase()
	}
}

// populateChoiceCaseResolvers iterates through the ChoiceCaseResolvers,
// retrieving the childs that nake up all the cases. per these childs
// (branches in the tree), the Highes precedence is being retrieved from the
// caches index (old intent content) as well as from the tree (new intent content).
// the choiceResolver is fed with the resulting values and thereby ready to be queried
// in a later stage (filterActiveChoiceCaseChilds()).
func (s *sharedEntryAttributes) populateChoiceCaseResolvers() {
	if s.schema == nil {
		return
	}
	// if choice/cases exist, process it
	for _, choiceResolver := range s.choicesResolvers {
		for _, elem := range choiceResolver.GetElementNames() {
			child, childExists := s.childs[elem]
			// Query the Index, stored in the treeContext for the per branch highes precedence
			if val := s.treeContext.GetBranchesHighesPrecedence(s.Path(), CacheUpdateFilterExcludeOwner(s.treeContext.GetActualOwner())); val < math.MaxInt32 {
				choiceResolver.SetValue(elem, val, false)
			}
			// set the value from the tree as well
			if childExists {
				v := child.GetHighestPrecedenceValueOfBranch()
				choiceResolver.SetValue(elem, v, true)
			}
		}
	}
}

// filterActiveChoiceCaseChilds returns the list of child elements. In case the Entry is
// a container with a / multiple choices, the list of childs is filtered to only return the
// cases that have the highest precedence.
func (s *sharedEntryAttributes) filterActiveChoiceCaseChilds() map[string]Entry {
	if s.schema == nil {
		return s.childs
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
	// if an entry for the given user exists, mark it for deletion
	if lvEntry != nil {
		lvEntry.MarkDelete()
	}
	// recurse into childs
	for _, child := range s.childs {
		child.MarkOwnerDelete(o)
	}
}

// AddCacheUpdateRecursive recursively adds the given cache.Update to the tree. Thereby creating all the entries along the path.
// if the entries along th path already exist, the existing entries are called to add the Update.
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

	return &RootEntry{
		sharedEntryAttributes: sea,
	}, nil
}

// String returns the string representation of the Tree.
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

// GetHighesPrecedence return the new cache.Update entried from the tree that are the highes priority.
// If the onlyNewOrUpdated option is set to true, only the New or Updated entries will be returned
// It will append to the given list and provide a new pointer to the slice
func (r *RootEntry) GetHighesPrecedence(onlyNewOrUpdated bool) UpdateSlice {
	return r.sharedEntryAttributes.GetHighestPrecedence(make(UpdateSlice, 0), onlyNewOrUpdated)
}

// GetDeletes returns the paths that due to the Tree content are to be deleted from the southbound device.
func (r *RootEntry) GetDeletes() [][]string {
	deletes := [][]string{}
	return r.sharedEntryAttributes.GetDeletes(deletes)
}

// GetTreeContext returns the handle to the TreeContext
func (r *RootEntry) GetTreeContext() *TreeContext {
	return r.treeContext
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
