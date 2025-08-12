package tree

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/tree/tree_persist"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"
)

// sharedEntryAttributes contains the attributes shared by Entry and RootEntry
type sharedEntryAttributes struct {
	// parent entry, nil for the root Entry
	parent Entry
	// pathElemName the path elements name the entry represents
	pathElemName string
	// childs mutual exclusive with LeafVariants
	childs      *childMap
	childsMutex sync.RWMutex
	// leafVariants mutual exclusive with Childs
	// If Entry is a leaf it can hold multiple leafVariants
	leafVariants *LeafVariants
	// schema the schema element for this entry
	schema      *sdcpb.SchemaElem
	schemaMutex sync.RWMutex

	choicesResolvers choiceResolvers

	treeContext *TreeContext

	// state cache
	cacheMutex        sync.Mutex
	cacheShouldDelete *bool
	cacheCanDelete    *bool
	cacheRemains      *bool
	level             *int
}

func (s *sharedEntryAttributes) deepCopy(tc *TreeContext, parent Entry) (*sharedEntryAttributes, error) {
	result := &sharedEntryAttributes{
		parent:           parent,
		pathElemName:     s.pathElemName,
		childs:           newChildMap(),
		schema:           s.schema,
		treeContext:      tc,
		choicesResolvers: s.choicesResolvers.deepCopy(),
		childsMutex:      sync.RWMutex{},
		schemaMutex:      sync.RWMutex{},
		cacheMutex:       sync.Mutex{},
		level:            s.level,
	}

	// copy childs
	for _, v := range s.childs.GetAll() {
		vCopy, err := v.DeepCopy(tc, result)
		if err != nil {
			return nil, err
		}
		result.childs.Add(vCopy)
	}

	// copy leafvariants
	result.leafVariants = s.leafVariants.DeepCopy(tc, result)

	return result, nil
}

func (s *sharedEntryAttributes) DeepCopy(tc *TreeContext, parent Entry) (Entry, error) {
	return s.deepCopy(tc, parent)
}

func newSharedEntryAttributes(ctx context.Context, parent Entry, pathElemName string, tc *TreeContext) (*sharedEntryAttributes, error) {
	s := &sharedEntryAttributes{
		parent:       parent,
		pathElemName: pathElemName,
		childs:       newChildMap(),
		treeContext:  tc,
	}
	s.leafVariants = newLeafVariants(tc, s)

	// populate the schema
	err := s.populateSchema(ctx)
	if err != nil {
		return nil, err
	}

	// try loading potential defaults
	err = s.loadDefaults(ctx)
	if err != nil {
		return nil, err
	}

	// initialize the choice case resolvers with the schema information
	s.initChoiceCasesResolvers()

	return s, nil
}

func (s *sharedEntryAttributes) GetRoot() Entry {
	if s.IsRoot() {
		return s
	}
	return s.parent.GetRoot()
}

// loadDefaults helper to populate defaults on the initializiation of the sharedEntryAttribute
func (s *sharedEntryAttributes) loadDefaults(ctx context.Context) error {

	// if it is a container without keys (not a list) then load the defaults
	if s.schema.GetContainer() != nil && len(s.schema.GetContainer().GetKeys()) == 0 {
		for _, childname := range s.schema.GetContainer().ChildsWithDefaults {
			// tryLoadingDefaults is using the pathslice, that contains keys as well,
			// so we need to make up for these extra entries in the slice
			path := append(s.Path(), childname)
			_, err := s.tryLoadingDefault(ctx, path)
			if err != nil {
				return err
			}
		}
	}

	// on the root element we cannot query the parent schema.
	// hence skip this part if IsRoot
	if !s.IsRoot() {

		// the regular container was covered further up, now we're after lists.
		// but only entries without a schema make sense to test. Since defaults
		// need to be added to the last level with no schema
		if s.schema != nil {
			return nil
		}

		// get the first ancestor with a schema and how many levels up that is
		ancestor, levelsUp := s.GetFirstAncestorWithSchema()

		// retrieve the container schema
		ancestorContainerSchema := ancestor.GetSchema().GetContainer()
		// if it is not a container, return
		if ancestorContainerSchema == nil {
			return nil
		}

		// if we're in the last level of keys, then we need to add the defaults
		if len(ancestorContainerSchema.Keys) == levelsUp {
			for _, childname := range ancestorContainerSchema.ChildsWithDefaults {
				path := append(s.Path(), childname)
				_, err := s.tryLoadingDefault(ctx, path)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *sharedEntryAttributes) GetDeviations(ch chan<- *types.DeviationEntry, activeCase bool) {
	// if s is a presence container but has active childs, it should not be treated as a presence
	// container, hence the leafvariants should not be processed. For presence container with
	// childs the TypedValue.empty_val in the presence container is irrelevant.
	if s.schema.GetContainer().GetIsPresence() && len(s.filterActiveChoiceCaseChilds()) > 0 {
		return
	}

	// calculate Deviation on the LeafVariants
	s.leafVariants.GetDeviations(ch, activeCase)

	// get all active childs
	activeChilds := s.filterActiveChoiceCaseChilds()

	// iterate through all childs
	for cName, c := range s.getChildren() {
		// check if c is a active child (choice / case)
		_, isActiveChild := activeChilds[cName]
		// recurse the call
		c.GetDeviations(ch, isActiveChild)
	}
}

func (s *sharedEntryAttributes) checkAndCreateKeysAsLeafs(ctx context.Context, intentName string, prio int32, insertFlag *types.UpdateInsertFlags) error {
	// keys themselfes do not have a schema attached.
	// keys must be added to the last keys level, since that is carrying the list elements data
	// hence if the entry has a schema attached, there is nothing to be done, return.
	if s.schema != nil {
		return nil
	}

	// get the first ancestor with a schema and how many levels up that is
	ancestor, levelsUp := s.GetFirstAncestorWithSchema()

	// retrieve the container schema
	ancestorContainerSchema := ancestor.GetSchema().GetContainer()
	// if it is not a container, return
	if ancestorContainerSchema == nil {
		return nil
	}

	// if we're in the last level of keys, then we need to add the defaults
	if len(ancestorContainerSchema.Keys) == levelsUp {
		// iterate through the keys
		schemaKeys := ancestor.GetSchemaKeys()
		slices.Sort(schemaKeys)
		for idx, k := range schemaKeys {
			child, entryExists := s.childs.GetEntry(k)
			// if the key Leaf exists continue with next key
			if entryExists {
				// if it exists, we need to check that the entry for the owner exists.
				var result []*LeafEntry
				lvs := child.GetByOwner(intentName, result)
				if len(lvs) > 0 {
					lvs[0].RemoveDeleteFlag()
					continue
				}
			}
			// construct the key path
			keyPath := append(s.Path(), k)

			schem, err := s.treeContext.schemaClient.GetSchemaSlicePath(ctx, keyPath)
			if err != nil {
				return err
			}
			// convert the key value to the schema defined Typed_Value
			tv, err := utils.Convert(keyPath[len(keyPath)-levelsUp-1+idx], schem.Schema.GetField().Type)
			if err != nil {
				return err
			}
			if !entryExists {
				// create a new entry
				child, err = newEntry(ctx, s, k, s.treeContext)
				if err != nil {
					return err
				}
				// add the new child entry to s
				err = s.addChild(ctx, child)
				if err != nil {
					return err
				}
			}
			_, err = child.AddUpdateRecursive(ctx, types.NewUpdate(keyPath, tv, prio, intentName, 0), insertFlag)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *sharedEntryAttributes) populateSchema(ctx context.Context) error {
	s.schemaMutex.Lock()
	defer s.schemaMutex.Unlock()

	getSchema := true

	// on the root element we cannot query the parent schema.
	// hence skip this part if IsRoot
	if !s.IsRoot() {

		// we can and should skip schema retrieval if we have a
		// terminal value that is a key value.
		// to check for that, we query the parent for the schema even multiple levels up
		// because we can have multiple keys. we remember the number of levels we moved up
		// and if that is within the len of keys, we're still in a key level, and need to skip
		// querying the schema. Otherwise we need to query the schema.
		ancesterschema, levelUp := s.GetFirstAncestorWithSchema()

		// check the found schema
		switch schem := ancesterschema.GetSchema().GetSchema().(type) {
		case *sdcpb.SchemaElem_Container:
			// if it is a container and level up is less or equal the levelUp count
			// this means, we are on a level this is for sure still a key level in the tree
			if len(schem.Container.GetKeys()) >= levelUp {
				getSchema = false
				break
			}
		}
	}

	if getSchema {
		// trieve if the getSchema var is still true
		schemaResp, err := s.treeContext.schemaClient.GetSchemaSlicePath(ctx, s.Path())
		if err != nil {
			return err
		}
		s.schema = schemaResp.GetSchema()
	}

	return nil
}

// GetSchema return the schema fiels of the Entry
func (s *sharedEntryAttributes) GetSchema() *sdcpb.SchemaElem {
	s.schemaMutex.RLock()
	defer s.schemaMutex.RUnlock()
	return s.schema
}

// GetChildren returns the children Map of the Entry
func (s *sharedEntryAttributes) getChildren() map[string]Entry {
	return s.childs.GetAll()
}

// getListChilds collects all the childs of the list. In the tree we store them seperated into their key branches.
// this is collecting all the last level key entries.
func (s *sharedEntryAttributes) GetListChilds() ([]Entry, error) {
	if s.schema == nil {
		return nil, fmt.Errorf("error GetListChilds() non schema level %s", s.Path().String())
	}
	if s.schema.GetContainer() == nil {
		return nil, fmt.Errorf("error GetListChilds() not a Container %s", s.Path().String())
	}
	keys := s.schema.GetContainer().GetKeys()
	if len(keys) == 0 {
		return nil, fmt.Errorf("error GetListChilds() not a List Container %s", s.Path().String())
	}
	actualEntries := []Entry{s}
	var newEntries []Entry

	for level := 0; level < len(keys); level++ {
		for _, e := range actualEntries {
			// add all children
			for _, c := range e.getChildren() {
				newEntries = append(newEntries, c)
			}
		}
		actualEntries = newEntries
		newEntries = []Entry{}
	}
	return actualEntries, nil

}

// FilterChilds returns the child entries (skipping the key entries in the tree) that
// match the given keys. The keys do not need to match all levels of keys, in which case the
// key level is considered a wildcard match (*)
func (s *sharedEntryAttributes) FilterChilds(keys map[string]string) ([]Entry, error) {
	if s.schema == nil {
		return nil, fmt.Errorf("error non schema level %s", s.Path().String())
	}

	result := []Entry{}
	// init the processEntries with s
	processEntries := []Entry{s}

	// retrieve the schema keys
	schemaKeys := s.GetSchemaKeys()
	// sort the keys, such that they appear in the order that they
	// are inserted in the tree
	sort.Strings(schemaKeys)
	// iterate through the keys, resolving the key levels
	for _, key := range schemaKeys {
		keyVal, exist := keys[key]
		// if the key exists in the input map meaning has a filter value
		// associated, the childs map is filtered for that value
		if exist {
			// therefor we need to go through the processEntries List
			// and collect all the matching childs
			for _, entry := range processEntries {
				childs := entry.getChildren()
				matchEntry, childExists := childs[keyVal]
				// so if such child, that matches the given filter value exists, we append it to the results
				if childExists {
					result = append(result, matchEntry)
				}
			}
		} else {
			// this is basically the wildcard case, so go through all childs and add them
			result = []Entry{}
			for _, entry := range processEntries {
				childs := entry.getChildren()
				for _, v := range childs {
					// hence we add all the existing childs to the result list
					result = append(result, v)
				}
			}
		}
		// prepare for the next iteration
		processEntries = result
	}
	return result, nil
}

// GetParent returns the parent entry
func (s *sharedEntryAttributes) GetParent() Entry {
	return s.parent
}

// IsRoot returns true if the element has no parent elements, hence is the root of the tree
func (s *sharedEntryAttributes) IsRoot() bool {
	return s.parent == nil
}

// GetLevel returns the level / depth position of this element in the tree
func (s *sharedEntryAttributes) GetLevel() int {
	// if level is cached, return level
	if s.level != nil {
		return *s.level
	}
	// if we're at the root level, return 0
	if s.IsRoot() {
		return 0
	}
	// Get parent level and add 1
	l := s.parent.GetLevel() + 1
	// cache level value
	s.level = &l
	return l
}

// Walk takes the EntryVisitor and applies it to every Entry in the tree
func (s *sharedEntryAttributes) Walk(ctx context.Context, v EntryVisitor) error {

	// execute the function locally
	err := v.Visit(ctx, s)
	if err != nil {
		return err
	}

	// trigger the execution on all childs
	for _, c := range s.childs.GetAll() {
		err := c.Walk(ctx, v)
		if err != nil {
			return err
		}
	}
	v.Up()
	return nil
}

// GetSchemaKeys checks for the schema of the entry, and returns the defined keys
func (s *sharedEntryAttributes) GetSchemaKeys() []string {
	if s.schema != nil {
		// if the schema is a container schema, we need to process the aggregation logic
		if contschema := s.schema.GetContainer(); contschema != nil {
			// if the level equals the amount of keys defined, we're at the right level, where the
			// actual elements start (not in a key level within the tree)
			var keys []string
			for _, k := range contschema.GetKeys() {
				keys = append(keys, k.Name)
			}
			return keys
		}
	}
	return nil
}

// getAggregatedDeletes is called on levels that have no schema attached, meaning key schemas.
// here we might delete the whole branch of the tree, if all key elements are being deleted
// if not, we continue with regular deltes
func (s *sharedEntryAttributes) getAggregatedDeletes(deletes []types.DeleteEntry, aggregatePaths bool) ([]types.DeleteEntry, error) {
	var err error
	// we take a look into the level(s) up
	// trying to get the schema
	ancestor, level := s.GetFirstAncestorWithSchema()

	// check if the first schema on the path upwards (parents)
	// has keys defined (meaning is a contianer with keys)
	keys := ancestor.GetSchemaKeys()

	// if keys exist and we're on the last level of the keys, validate
	// if aggregation can happen
	if len(keys) > 0 && level == len(keys) {
		doAggregateDelete := true
		// check the keys for deletion
		for _, n := range keys {
			c, exists := s.childs.GetEntry(n)
			// these keys should aways exist, so for now we do not catch the non existing key case
			if exists && !c.shouldDelete() {
				// if not all the keys are marked for deletion, we need to revert to regular deletion
				doAggregateDelete = false
				break
			}
		}
		// if aggregate delet is possible do it
		if doAggregateDelete {
			// by adding the key path to the deletes
			deletes = append(deletes, s)
		} else {
			// otherwise continue with deletion on the childs.
			for _, c := range s.childs.GetAll() {
				deletes, err = c.GetDeletes(deletes, aggregatePaths)
				if err != nil {
					return nil, err
				}
			}
		}
		return deletes, nil
	}
	return s.getRegularDeletes(deletes, aggregatePaths)
}

// canDelete checks if the entry can be Deleted.
// This is e.g. to cover e.g. defaults and running. They can be deleted, but should not, they are basically implicitly existing.
// In caomparison to
//   - remainsToExists() returns true, because they remain to exist even though implicitly.
//   - shouldDelete() returns false, because no explicit delete should be issued for them.
func (s *sharedEntryAttributes) canDelete() bool {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	if s.cacheCanDelete != nil {
		return *s.cacheCanDelete
	}

	leafVariantCanDelete := s.leafVariants.canDelete()
	if !leafVariantCanDelete {
		s.cacheCanDelete = utils.BoolPtr(false)
		return *s.cacheCanDelete
	}

	// handle containers
	for _, c := range s.filterActiveChoiceCaseChilds() {
		canDelete := c.canDelete()
		if !canDelete {
			s.cacheCanDelete = utils.BoolPtr(false)
			return *s.cacheCanDelete
		}
	}
	s.cacheCanDelete = utils.BoolPtr(true)
	return *s.cacheCanDelete
}

func (s *sharedEntryAttributes) canDeleteBranch(keepDefault bool) bool {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	leafVariantCanDelete := s.leafVariants.canDeleteBranch(keepDefault)
	if !leafVariantCanDelete {
		return false
	}

	// handle containers
	for _, c := range s.childs.Items() {
		canDelete := c.canDeleteBranch(keepDefault)
		if !canDelete {
			return false
		}
	}

	return true
}

// shouldDelete checks if a container or Leaf(List) is to be explicitly deleted.
func (s *sharedEntryAttributes) shouldDelete() bool {
	// see if we have the value cached
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	if s.cacheShouldDelete != nil {
		return *s.cacheShouldDelete
	}
	// check if the leafVariants result in a should delete
	leafVariantshouldDelete := s.leafVariants.shouldDelete()

	// for containers, it is basically a canDelete() check.
	canDelete := false
	// but a real delete should only be added if there is at least one shouldDelete() == true
	shouldDelete := false

	activeChilds := s.filterActiveChoiceCaseChilds()
	// if we have no active childs, we can and should delete.
	if len(s.choicesResolvers) > 0 && len(activeChilds) == 0 {
		canDelete = true
		shouldDelete = true
	}

	// iterate through the active childs
	for _, c := range activeChilds {
		// check if the child can be deleted
		canDelete = c.canDelete()
		// if it can explicitly not be deleted, then the result is clear, we should not delete
		if !canDelete {
			break
		}
		// if it can be deleted we need to check if there is a contibuting entry that
		// requires deletion only if there is a contributing shouldDelete() == true then we must issue
		// a real delete
		shouldDelete = shouldDelete || c.shouldDelete()
	}

	// the overall result is
	// if we have a leaf
	//     the result of the leafVariant.shouldDelete() calculation
	// or if it is a conmtainer
	//     canDelete() [if no canDelete() == true then it must remain]
	// 	 and
	//     shouldDelete() [only if an entry is explicitly to be deleted, issue a delete]
	//   and
	//     s.leafVariants.canDelete()
	result := leafVariantshouldDelete || (canDelete && shouldDelete && s.leafVariants.canDelete())

	s.cacheShouldDelete = &result
	return result
}

func (s *sharedEntryAttributes) remainsToExist() bool {
	// see if we have the value cached
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	if s.cacheRemains != nil {
		return *s.cacheRemains
	}
	leafVariantResult := s.leafVariants.remainsToExist()

	// handle containers
	childsRemain := false
	for _, c := range s.filterActiveChoiceCaseChilds() {
		childsRemain = c.remainsToExist()
		if childsRemain {
			break
		}
	}

	// assumption is, that if the entry exists, there is at least a running value available.
	remains := leafVariantResult || childsRemain

	s.cacheRemains = &remains
	return remains
}

// getRegularDeletes performs deletion calculation on elements that have a schema attached.
func (s *sharedEntryAttributes) getRegularDeletes(deletes []types.DeleteEntry, aggregate bool) ([]types.DeleteEntry, error) {
	var err error

	if s.shouldDelete() && !s.IsRoot() && len(s.GetSchemaKeys()) == 0 {
		return append(deletes, s), nil
	}

	for _, elem := range s.choicesResolvers.GetDeletes() {
		path, err := s.SdcpbPath()
		if err != nil {
			return nil, err
		}
		path.Elem = append(path.Elem, &sdcpb.PathElem{Name: elem})
		deletes = append(deletes, types.NewDeleteEntryImpl(path))
	}

	for _, e := range s.childs.GetAll() {
		deletes, err = e.GetDeletes(deletes, aggregate)
		if err != nil {
			return nil, err
		}
	}
	return deletes, nil
}

// GetDeletes calculate the deletes that need to be send to the device.
func (s *sharedEntryAttributes) GetDeletes(deletes []types.DeleteEntry, aggregatePaths bool) ([]types.DeleteEntry, error) {

	// if the actual level has no schema assigned we're on a key level
	// element. Hence we try deletion via aggregation
	if s.schema == nil && aggregatePaths {
		return s.getAggregatedDeletes(deletes, aggregatePaths)
	}

	// else perform regular deletion
	return s.getRegularDeletes(deletes, aggregatePaths)
}

// GetAncestorSchema returns the schema of the parent node if the schema is set.
// if the parent has no schema (is a key element in the tree) it will recurs the call to the parents parent.
// the level of recursion is indicated via the levelUp attribute
func (s *sharedEntryAttributes) GetFirstAncestorWithSchema() (Entry, int) {
	// if root node is reached
	if s.IsRoot() {
		return nil, 0
	}
	// check if the parent has a schema
	if s.parent.GetSchema() != nil {
		// if so return it with level 1
		return s.parent, 1
	}
	// direct parent does not have a schema, recurse the call
	schema, level := s.parent.GetFirstAncestorWithSchema()
	// increase the level returned by the parent to
	// reflect this entry as a level and return
	return schema, level + 1
}

// GetByOwner returns all the LeafEntries that belong to a certain owner.
func (s *sharedEntryAttributes) GetByOwner(owner string, result []*LeafEntry) LeafVariantSlice {
	lv := s.leafVariants.GetByOwner(owner)
	if lv != nil {
		result = append(result, lv)
	}

	// continue with childs
	for _, c := range s.childs.GetAll() {
		result = c.GetByOwner(owner, result)
	}
	return result
}

// Path returns the root based path of the Entry
func (s *sharedEntryAttributes) Path() types.PathSlice {
	// special handling for root node
	if s.IsRoot() {
		return types.PathSlice{}
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

// addChild add an entry to the list of child entries for the entry.
func (s *sharedEntryAttributes) addChild(ctx context.Context, e Entry) error {
	// make sure Entry should not only hold LeafEntries
	if s.leafVariants.Length() > 0 {
		// An exception are presence containers
		_, is_container := s.schema.Schema.(*sdcpb.SchemaElem_Container)
		if !is_container && !s.schema.GetContainer().IsPresence {
			return fmt.Errorf("cannot add child to %s since it holds Leafs", s)
		}
	}
	// check the path of child is a subpath of s
	if !slices.Equal(s.Path(), e.Path()[:len(e.Path())-1]) {
		return fmt.Errorf("adding Child with diverging path, parent: %s, child: %s", s, strings.Join(e.Path()[:len(e.Path())-1], "/"))
	}
	s.childs.Add(e)
	return nil
}

func (s *sharedEntryAttributes) NavigateSdcpbPath(ctx context.Context, pathElems []*sdcpb.PathElem, isRootPath bool) (Entry, error) {
	var err error
	if len(pathElems) == 0 {
		return s, nil
	}

	if isRootPath {
		return s.GetRoot().NavigateSdcpbPath(ctx, pathElems, false)
	}

	switch pathElems[0].Name {
	case ".":
		s.NavigateSdcpbPath(ctx, pathElems[1:], false)
	case "..":
		var entry Entry
		entry = s.parent
		// we need to skip key levels in the tree
		// if the next path element is again .. we need to skip key values that are present in the tree
		// If it is a sub-entry instead, we need to stay in the brach that is defined by the key values
		// hence only delegate the call to the parent

		if len(pathElems) > 1 && pathElems[1].Name == ".." {
			entry, _ = s.GetFirstAncestorWithSchema()
		}
		return entry.NavigateSdcpbPath(ctx, pathElems[1:], false)
	default:
		e, exists := s.filterActiveChoiceCaseChilds()[pathElems[0].Name]
		if !exists {
			pth := &sdcpb.Path{Elem: pathElems}
			e, err = s.tryLoadingDefault(ctx, utils.ToStrings(pth, false, false))
			if err != nil {
				pathStr := utils.ToXPath(pth, false)
				return nil, fmt.Errorf("navigating tree, reached %v but child %v does not exist, trying to load defaults yielded %v", s.Path(), pathStr, err)
			}
			return e, nil
		}

		for _, v := range pathElems[0].Key {
			e, err = e.Navigate(ctx, []string{v}, false, false)
			if err != nil {
				return nil, err
			}
		}

		return e.NavigateSdcpbPath(ctx, pathElems[1:], false)
	}

	return nil, fmt.Errorf("navigating tree, reached %v but child %v does not exist", s.Path(), pathElems)
}

func (s *sharedEntryAttributes) tryLoadingDefault(ctx context.Context, path []string) (Entry, error) {

	schema, err := s.treeContext.schemaClient.GetSchemaSlicePath(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("error trying to load defaults for %s: %v", strings.Join(path, "->"), err)
	}

	upd, err := DefaultValueRetrieve(schema.GetSchema(), path, DefaultValuesPrio, DefaultsIntentName)
	if err != nil {
		return nil, err
	}

	flags := types.NewUpdateInsertFlags()

	result, err := s.AddUpdateRecursive(ctx, upd, flags)
	if err != nil {
		return nil, fmt.Errorf("failed adding default value for %s to tree; %v", strings.Join(path, "/"), err)
	}

	return result, nil
}

// Navigate move through the tree, returns the Entry that is present under the given path
func (s *sharedEntryAttributes) Navigate(ctx context.Context, path []string, isRootPath bool, dotdotSkipKeys bool) (Entry, error) {
	if len(path) == 0 {
		return s, nil
	}

	if isRootPath {
		return s.treeContext.root.Navigate(ctx, path, false, dotdotSkipKeys)
	}
	var err error
	switch path[0] {
	case ".":
		return s.Navigate(ctx, path[1:], false, dotdotSkipKeys)
	case "..":
		parent := s.parent
		if dotdotSkipKeys {
			// if dotdotSkipKeys is set, we need to advance to the next schema level above
			// the issue is, that if there is a list, with keys, the last element even without a schema defined
			// is the element to stop at.
			// so here we need to check is the parent schema is nil and that we still want to move further up.
			// if thats the case, move up to the next schema carrying element.
			if parent.GetSchema() == nil && len(path) > 0 && path[1] == ".." {
				parent, _ = parent.GetFirstAncestorWithSchema()
			}
		}
		return parent.Navigate(ctx, path[1:], false, dotdotSkipKeys)
	default:
		e, exists := s.filterActiveChoiceCaseChilds()[path[0]]
		if !exists {
			e, err = s.tryLoadingDefault(ctx, append(s.Path(), path...))
			if err != nil {
				return nil, fmt.Errorf("navigating tree, reached %v but child %v does not exist, trying to load defaults yielded %v", s.Path(), path, err)
			}
			return e, nil
		}
		return e.Navigate(ctx, path[1:], false, dotdotSkipKeys)
	}
}

func (s *sharedEntryAttributes) DeleteBranch(ctx context.Context, relativePath types.PathSlice, owner string) error {
	var err error
	var entry Entry
	if len(relativePath) > 0 {
		entry, err = s.Navigate(ctx, relativePath, true, false)
		if err != nil {
			return err
		}

		err = entry.DeleteBranch(ctx, nil, owner)
		if err != nil {
			return err
		}

		// need to remove the leafvariants down from entry.
		// however if the path points to a key, which is in fact getting deleted
		// we also need to remove the key, which is the parent. Thats why we do it in this loop
		// which is, forwarding entry to entry.GetParent() as a last step and depending on the remains
		// return continuing to perform the delete forther up in the tree
		// with remains initially set to false, we initially call DeleteSubtree on the referenced entry.
		for entry.canDeleteBranch(false) {
			// forward the entry pointer to the parent
			// depending on the remains var the DeleteSubtree is again called on that parent entry
			entry = entry.GetParent()
			// calling DeleteSubtree with the empty string, because it should not delete the owner from the higher level keys,
			// but what it will also do is delete possibly dangling key elements in the tree
			entry.deleteCanDeleteChilds(true)
		}
		return nil
	}
	return s.deleteBranchInternal(ctx, owner)
}

func (s *sharedEntryAttributes) deleteCanDeleteChilds(keepDefault bool) {
	// otherwise check all
	for childname, child := range s.childs.Items() {
		if child.canDeleteBranch(keepDefault) {
			s.childs.DeleteChild(childname)
		}
	}
}

func (s *sharedEntryAttributes) deleteBranchInternal(ctx context.Context, owner string) error {
	// delete possibly existing leafvariants for the owner
	s.leafVariants.DeleteByOwner(owner)

	// recurse the call
	for childName, child := range s.childs.Items() {
		err := child.DeleteBranch(ctx, nil, owner)
		if err != nil {
			return err
		}
		if child.canDeleteBranch(false) {
			s.childs.DeleteChild(childName)
		}
	}
	return nil
}

// GetHighestPrecedence goes through the whole branch and returns the new and updated cache.Updates.
// These are the updated that will be send to the device.
func (s *sharedEntryAttributes) GetHighestPrecedence(result LeafVariantSlice, onlyNewOrUpdated bool, includeDefaults bool) LeafVariantSlice {
	// get the highes precedence LeafeVariant and add it to the list
	lv := s.leafVariants.GetHighestPrecedence(onlyNewOrUpdated, includeDefaults)
	if lv != nil {
		result = append(result, lv)
	}

	// continue with childs. Childs are part of choices, process only the "active" (highes precedence) childs
	for _, c := range s.filterActiveChoiceCaseChilds() {
		result = c.GetHighestPrecedence(result, onlyNewOrUpdated, includeDefaults)
	}
	return result
}

func (s *sharedEntryAttributes) getHighestPrecedenceLeafValue(ctx context.Context) (*LeafEntry, error) {
	for _, x := range []string{"existing", "default"} {
		lv := s.leafVariants.GetHighestPrecedence(false, true)
		if lv != nil {
			return lv, nil
		}
		if x != "default" {
			_, err := s.tryLoadingDefault(ctx, s.Path())
			if err != nil {
				return nil, err
			}
		}
	}
	return nil, fmt.Errorf("error no value present for %s", s.Path())
}

func (s *sharedEntryAttributes) GetRootBasedEntryChain() []Entry {
	if s.IsRoot() {
		return []Entry{}
	}
	return append(s.parent.GetRootBasedEntryChain(), s)
}

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

// getHighestPrecedenceValueOfBranch goes through all the child branches to find the highest
// precedence value (lowest priority value) for the entire branch and returns it.
func (s *sharedEntryAttributes) getHighestPrecedenceValueOfBranch(filter HighestPrecedenceFilter) int32 {
	result := int32(math.MaxInt32)
	for _, e := range s.childs.GetAll() {
		if val := e.getHighestPrecedenceValueOfBranch(filter); val < result {
			result = val
		}
	}
	if val := s.leafVariants.GetHighestPrecedenceValue(filter); val < result {
		result = val
	}

	return result
}

// Validate is the highlevel function to perform validation.
// it will multiplex all the different Validations that need to happen
func (s *sharedEntryAttributes) Validate(ctx context.Context, resultChan chan<- *types.ValidationResultEntry, statChan chan<- *types.ValidationStat, vCfg *config.Validation) {

	// recurse the call to the child elements
	wg := sync.WaitGroup{}
	defer wg.Wait()
	for _, c := range s.filterActiveChoiceCaseChilds() {
		wg.Add(1)
		valFunc := func(x Entry) {
			x.Validate(ctx, resultChan, statChan, vCfg)
			wg.Done()
		}
		if !vCfg.DisableConcurrency {
			go valFunc(c)
		} else {
			valFunc(c)
		}
	}

	// validate the mandatory statement on this entry
	if s.remainsToExist() {

		// TODO: Validate Enums

		if !vCfg.DisabledValidators.Mandatory {
			s.validateMandatory(ctx, resultChan, statChan)
		}
		if !vCfg.DisabledValidators.Leafref {
			s.validateLeafRefs(ctx, resultChan, statChan)
		}
		if !vCfg.DisabledValidators.LeafrefMinMaxAttributes {
			s.validateLeafListMinMaxAttributes(resultChan, statChan)
		}
		if !vCfg.DisabledValidators.Pattern {
			s.validatePattern(resultChan, statChan)
		}
		if !vCfg.DisabledValidators.MustStatement {
			s.validateMustStatements(ctx, resultChan, statChan)
		}
		if !vCfg.DisabledValidators.Length {
			s.validateLength(resultChan, statChan)
		}
		if !vCfg.DisabledValidators.Range {
			s.validateRange(resultChan, statChan)
		}
		//if !vCfg.DisabledValidators.MaxElements {
		//	s.validateMaxElements(resultChan, statChan)
		//}
	}
}

// validateRange int and uint types (Leaf and Leaflist) define ranges which configured values must lay in.
// validateRange does check this condition.
func (s *sharedEntryAttributes) validateRange(resultChan chan<- *types.ValidationResultEntry, statChan chan<- *types.ValidationStat) {

	// if no schema present or Field and LeafList Types do not contain any ranges, return there is nothing to check
	if s.GetSchema() == nil || (len(s.GetSchema().GetField().GetType().GetRange()) == 0 && len(s.GetSchema().GetLeaflist().GetType().GetRange()) == 0) {
		return
	}

	lv := s.leafVariants.GetHighestPrecedence(false, true)
	if lv == nil {
		return
	}

	tv := lv.Update.Value()

	var tvs []*sdcpb.TypedValue
	var typeSchema *sdcpb.SchemaLeafType
	// ranges are defined on Leafs or LeafLists.
	switch {
	case len(s.GetSchema().GetField().GetType().GetRange()) != 0:
		// if it is a leaf, extract the value add it as a single value to the tvs slice and check it further down
		tvs = []*sdcpb.TypedValue{tv}
		// we also need the Field/Leaf Type schema
		typeSchema = s.GetSchema().GetField().GetType()
	case len(s.GetSchema().GetLeaflist().GetType().GetRange()) != 0:
		// if it is a leaflist, extract the values them to the tvs slice and check them further down
		tvs = tv.GetLeaflistVal().GetElement()
		// we also need the Field/Leaf Type schema
		typeSchema = s.GetSchema().GetLeaflist().GetType()
	default:
		// if no ranges exist return
		return
	}

	stat := types.NewValidationStat(types.StatTypeRange)

	// range through the tvs and check that they are in range
	for _, tv := range tvs {
		// we need to distinguish between unsigned and singned ints
		switch typeSchema.TypeName {
		case "uint8", "uint16", "uint32", "uint64":
			// procede with the unsigned ints
			urnges := utils.NewUrnges()
			// add the defined ranges to the ranges struct
			for _, r := range typeSchema.GetRange() {
				stat.PlusOne()
				urnges.AddRange(r.Min.Value, r.Max.Value)
			}

			// check the value laays within any of the ranges
			if !urnges.IsWithinAnyRange(tv.GetUintVal()) {
				resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("path %s, value %d not within any of the expected ranges %s", s.Path(), tv.GetUintVal(), urnges.String()), types.ValidationResultEntryTypeError)
			}

		case "int8", "int16", "int32", "int64":
			// procede with the signed ints
			srnges := utils.NewSrnges()
			for _, r := range typeSchema.GetRange() {
				stat.PlusOne()
				// get the value
				min := int64(r.GetMin().GetValue())
				max := int64(r.GetMax().GetValue())
				// take care of the minus sign
				if r.Min.Negative {
					min = min * -1
				}
				if r.Max.Negative {
					max = max * -1
				}
				// add the defined ranges to the ranges struct
				srnges.AddRange(min, max)
			}
			// check the value laays within any of the ranges
			if !srnges.IsWithinAnyRange(tv.GetIntVal()) {
				resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("path %s, value %d not within any of the expected ranges %s", s.Path(), tv.GetIntVal(), srnges.String()), types.ValidationResultEntryTypeError)
			}
		}
	}
	statChan <- stat
}

// validateLeafListMinMaxAttributes validates the Min-, and Max-Elements attribute of the Entry if it is a Leaflists.
func (s *sharedEntryAttributes) validateLeafListMinMaxAttributes(resultChan chan<- *types.ValidationResultEntry, statChan chan<- *types.ValidationStat) {
	if schema := s.schema.GetLeaflist(); schema != nil {
		if schema.GetMinElements() > 0 || schema.GetMaxElements() < math.MaxUint64 {
			if lv := s.leafVariants.GetHighestPrecedence(false, true); lv != nil {
				tv := lv.Update.Value()

				if val := tv.GetLeaflistVal(); val != nil {
					// check minelements if set
					if schema.GetMinElements() > 0 && len(val.GetElement()) < int(schema.GetMinElements()) {
						resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("leaflist %s defines %d min-elements but only %d elements are present", s.Path().String(), schema.MinElements, len(val.GetElement())), types.ValidationResultEntryTypeError)
					}
					// check maxelements if set
					if uint64(len(val.GetElement())) > uint64(schema.GetMaxElements()) {
						resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("leaflist %s defines %d max-elements but %d elements are present", s.Path().String(), schema.GetMaxElements(), len(val.GetElement())), types.ValidationResultEntryTypeError)
					}
				}
				statChan <- types.NewValidationStat(types.StatTypeMinMax).PlusOne()
			}
		}
	}
}

func (s *sharedEntryAttributes) validateLength(resultChan chan<- *types.ValidationResultEntry, statChan chan<- *types.ValidationStat) {
	if schema := s.schema.GetField(); schema != nil {

		if len(schema.GetType().GetLength()) == 0 {
			return
		}

		stat := types.NewValidationStat(types.StatTypeLength)

		lv := s.leafVariants.GetHighestPrecedence(false, true)
		if lv == nil {
			return
		}
		value := lv.Value().GetStringVal()
		actualLength := utf8.RuneCountInString(value)

		for _, lengthDef := range schema.GetType().GetLength() {
			stat.PlusOne()
			if lengthDef.Min.Value <= uint64(actualLength) && uint64(actualLength) <= lengthDef.Max.Value {
				// continue if the length is within the range
				continue
			}
			// this is already the failure case
			lenghts := []string{}
			for _, lengthDef := range schema.GetType().GetLength() {
				lenghts = append(lenghts, fmt.Sprintf("%d..%d", lengthDef.Min.Value, lengthDef.Max.Value))
			}
			resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("error length of Path: %s, Value: %s not within allowed length %s", s.Path(), value, strings.Join(lenghts, ", ")), types.ValidationResultEntryTypeError)
		}
		statChan <- stat
	}
}

func (s *sharedEntryAttributes) validatePattern(resultChan chan<- *types.ValidationResultEntry, statChan chan<- *types.ValidationStat) {
	if schema := s.schema.GetField(); schema != nil {
		if len(schema.Type.Patterns) == 0 {
			return
		}
		stat := types.NewValidationStat(types.StatTypePattern)
		lv := s.leafVariants.GetHighestPrecedence(false, true)
		value := lv.Value().GetStringVal()
		for _, pattern := range schema.Type.Patterns {
			if p := pattern.GetPattern(); p != "" {
				matched, err := regexp.MatchString(p, value)
				if err != nil {
					resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("failed compiling regex %s defined for %s", p, s.Path()), types.ValidationResultEntryTypeError)
					continue
				}
				if (!matched && !pattern.Inverted) || (pattern.GetInverted() && matched) {
					resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("value %s of %s does not match regex %s (inverted: %t)", value, s.Path(), p, pattern.GetInverted()), types.ValidationResultEntryTypeError)
				}
			}
			stat.PlusOne()
		}
		statChan <- stat
	}
}

func (s *sharedEntryAttributes) ImportConfig(ctx context.Context, t importer.ImportConfigAdapter, intentName string, intentPrio int32, insertFlags *types.UpdateInsertFlags) error {
	var err error

	switch x := s.schema.GetSchema().(type) {
	case *sdcpb.SchemaElem_Container, nil:
		switch {
		case len(s.schema.GetContainer().GetKeys()) > 0:

			var exists bool
			var actualEntry Entry = s
			var keyChild Entry
			schemaKeys := s.GetSchemaKeys()
			slices.Sort(schemaKeys)
			for _, schemaKey := range schemaKeys {

				keyTransf := t.GetElement(schemaKey)
				if keyTransf == nil {
					return fmt.Errorf("unable to find key attribute %s under %s", schemaKey, s.Path())
				}
				keyElemValue, err := keyTransf.GetKeyValue()
				if err != nil {
					return err
				}
				// if the child does not exist, create it
				if keyChild, exists = actualEntry.getChildren()[keyElemValue]; !exists {
					keyChild, err = newEntry(ctx, actualEntry, keyElemValue, s.treeContext)
					if err != nil {
						return err
					}
				}
				actualEntry = keyChild
			}
			err = actualEntry.ImportConfig(ctx, t, intentName, intentPrio, insertFlags)
			if err != nil {
				return err
			}
		default:
			if len(t.GetElements()) == 0 {
				// it might be a presence container
				schem := s.schema.GetContainer()
				if schem == nil {
					return nil
				}
				if schem.IsPresence {
					tv := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_EmptyVal{EmptyVal: &emptypb.Empty{}}}
					upd := types.NewUpdate(s.Path(), tv, intentPrio, intentName, 0)
					s.leafVariants.Add(NewLeafEntry(upd, insertFlags, s))
				}
			}

			for _, elem := range t.GetElements() {
				var child Entry
				var exists bool

				// if the child does not exist, create it
				if child, exists = s.getChildren()[elem.GetName()]; !exists {
					child, err = newEntry(ctx, s, elem.GetName(), s.treeContext)
					if err != nil {
						return fmt.Errorf("error trying to insert %s at path %v: %w", elem.GetName(), s.Path(), err)
					}
					err = s.addChild(ctx, child)
					if err != nil {
						return err
					}
				}
				err = child.ImportConfig(ctx, elem, intentName, intentPrio, insertFlags)
				if err != nil {
					return err
				}
			}
		}
	case *sdcpb.SchemaElem_Field:
		// // if it is as leafref we need to figure out the type of the references field.
		fieldType := x.Field.GetType()
		// if x.Field.GetType().Type == "leafref" {
		// 	s.treeContext.treeSchemaCacheClient.GetSchema(ctx,)
		// }

		tv, err := t.GetTVValue(fieldType)
		if err != nil {
			return err
		}
		upd := types.NewUpdate(s.Path(), tv, intentPrio, intentName, 0)

		s.leafVariants.Add(NewLeafEntry(upd, insertFlags, s))

	case *sdcpb.SchemaElem_Leaflist:
		var scalarArr *sdcpb.ScalarArray
		mustAdd := false
		le := s.leafVariants.GetByOwner(intentName)
		if le != nil {
			scalarArr = le.Value().GetLeaflistVal()
		} else {
			le = NewLeafEntry(nil, insertFlags, s)
			mustAdd = true
			scalarArr = &sdcpb.ScalarArray{Element: []*sdcpb.TypedValue{}}
		}

		tv, err := t.GetTVValue(x.Leaflist.GetType())
		if err != nil {
			return err
		}

		// the proto implementation will return leaflist tvs
		if tv.GetLeaflistVal() == nil {
			scalarArr.Element = append(scalarArr.Element, tv)
			tv = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_LeaflistVal{LeaflistVal: scalarArr}}
		}

		le.Update = types.NewUpdate(s.Path(), tv, intentPrio, intentName, 0)
		if mustAdd {
			s.leafVariants.Add(le)
		}
	}
	return nil
}

// validateMandatory validates that all the mandatory attributes,
// defined by the schema are present either in the tree or in the index.
func (s *sharedEntryAttributes) validateMandatory(ctx context.Context, resultChan chan<- *types.ValidationResultEntry, statChan chan<- *types.ValidationStat) {
	if s.shouldDelete() {
		return
	}
	if s.schema != nil {
		switch s.schema.GetSchema().(type) {
		case *sdcpb.SchemaElem_Container:
			stat := types.NewValidationStat(types.StatTypeMandatory)
			for _, c := range s.schema.GetContainer().GetMandatoryChildrenConfig() {
				stat.PlusOne()
				attributes := []string{}
				choiceName := ""
				// check if it is a ChildContainer
				if slices.Contains(s.schema.GetContainer().GetChildren(), c.Name) {
					attributes = append(attributes, c.Name)
				}

				// check if it is a Field
				if slices.ContainsFunc(s.schema.GetContainer().GetFields(), func(x *sdcpb.LeafSchema) bool {
					return x.Name == c.Name
				}) {
					attributes = append(attributes, c.Name)
				}

				// otherwise it will probably be a choice
				if len(attributes) == 0 {
					choice := s.schema.GetContainer().GetChoiceInfo().GetChoiceByName(c.Name)
					if choice != nil {
						attributes = append(attributes, choice.GetAllAttributes()...)
						choiceName = c.Name
					}
				}

				if len(attributes) == 0 {
					log.Errorf("error path: %s, validationg mandatory attribute %s could not be found as child, field or choice.", s.Path(), c.Name)
				}

				s.validateMandatoryWithKeys(ctx, len(s.GetSchema().GetContainer().GetKeys()), attributes, choiceName, resultChan)
			}
			statChan <- stat
		}
	}
}

// validateMandatoryWithKeys steps down the tree, passing the key levels and checking the existence of the mandatory.
// attributes is a string slice, it will be checked that at least of the the given attributes is defined
// !Not checking all of these are defined (call multiple times with single entry in attributes for that matter)!
func (s *sharedEntryAttributes) validateMandatoryWithKeys(ctx context.Context, level int, attributes []string, choiceName string, resultChan chan<- *types.ValidationResultEntry) {
	if s.shouldDelete() {
		return
	}
	if level == 0 {
		success := false
		existsInTree := false
		var v Entry
		// iterate over the attributes make sure any of these exists
		for _, attr := range attributes {
			// first check if the mandatory value is set via the intent, e.g. part of the tree already
			v, existsInTree = s.filterActiveChoiceCaseChilds()[attr]
			// if exists and remains to Exist
			if existsInTree && v.remainsToExist() {
				// set success to true and break the loop
				success = true
				break
			}
		}
		// if not the path exists in the tree and is not to be deleted, then lookup in the paths index of the store
		// and see if such path exists, if not raise the error
		if !success {
			// if it is not a choice
			if choiceName == "" {
				resultChan <- types.NewValidationResultEntry("unknown", fmt.Errorf("error mandatory child %s does not exist, path: %s", attributes, s.Path()), types.ValidationResultEntryTypeError)
				return
			}
			// if it is a mandatory choice
			resultChan <- types.NewValidationResultEntry("unknown", fmt.Errorf("error mandatory choice %s [attributes: %s] does not exist, path: %s", choiceName, attributes, s.Path()), types.ValidationResultEntryTypeError)
			return
		}
		return
	}

	for _, c := range s.filterActiveChoiceCaseChilds() {
		c.validateMandatoryWithKeys(ctx, level-1, attributes, choiceName, resultChan)
	}
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
	choicesResolvers := choiceResolvers{}

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
func (s *sharedEntryAttributes) FinishInsertionPhase(ctx context.Context) error {

	// populate the ChoiceCaseResolvers to determine the active case
	err := s.populateChoiceCaseResolvers(ctx)
	if err != nil {
		return err
	}

	// recurse the call to all (active) entries within the tree.
	// Thereby already using the choiceCaseResolver via filterActiveChoiceCaseChilds()
	for _, child := range s.filterActiveChoiceCaseChilds() {
		err = child.FinishInsertionPhase(ctx)
		if err != nil {
			return err
		}
	}

	// reset state
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	s.cacheRemains = nil
	s.cacheShouldDelete = nil
	s.cacheCanDelete = nil

	return nil
}

// populateChoiceCaseResolvers iterates through the ChoiceCaseResolvers,
// retrieving the childs that nake up all the cases. per these childs
// (branches in the tree), the Highes precedence is being retrieved from the
// caches index (old intent content) as well as from the tree (new intent content).
// the choiceResolver is fed with the resulting values and thereby ready to be queried
// in a later stage (filterActiveChoiceCaseChilds()).
func (s *sharedEntryAttributes) populateChoiceCaseResolvers(_ context.Context) error {
	if s.schema == nil {
		return nil
	}
	// if choice/cases exist, process it
	for _, choiceResolver := range s.choicesResolvers {
		for _, elem := range choiceResolver.GetElementNames() {
			isDeleted := false
			highestWDeleted := int32(math.MaxInt32)
			highestWODeleted := int32(math.MaxInt32)
			highestWONew := int32(math.MaxInt32)

			child, childExists := s.childs.GetEntry(elem)
			// set the value from the tree as well
			if childExists {
				valWDeleted := child.getHighestPrecedenceValueOfBranch(HighestPrecedenceFilterAll)
				if valWDeleted <= highestWDeleted {
					highestWDeleted = valWDeleted
					if child.canDelete() {
						isDeleted = true
					}
				}

				valWODeleted := child.getHighestPrecedenceValueOfBranch(HighestPrecedenceFilterWithoutDeleted)
				if valWODeleted <= highestWODeleted {
					highestWODeleted = valWODeleted
				}
				valWONew := child.getHighestPrecedenceValueOfBranch(HighestPrecedenceFilterWithoutNew)
				if valWONew <= highestWONew {
					highestWONew = valWONew
				}

			}
			choiceResolver.SetValue(elem, highestWODeleted, highestWDeleted, highestWONew, isDeleted)
		}
	}
	return nil
}

// filterActiveChoiceCaseChilds returns the list of child elements. In case the Entry is
// a container with a / multiple choices, the list of childs is filtered to only return the
// cases that have the highest precedence.
func (s *sharedEntryAttributes) filterActiveChoiceCaseChilds() map[string]Entry {
	if s.schema == nil {
		return s.childs.GetAll()
	}

	skipAttributesList := s.choicesResolvers.GetSkipElements()
	// if there are no items that should be skipped, take a shortcut
	// and simply return all childs straight away
	if len(skipAttributesList) == 0 {
		return s.childs.GetAll()
	}
	result := map[string]Entry{}
	// optimization option: sort the slices and forward in parallel, lifts extra burden that the contains call holds.
	for childName, child := range s.childs.GetAll() {
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
	for _, child := range s.childs.GetAllSorted() {
		result = child.StringIndent(result)
	}
	// range over LeafVariants
	for l := range s.leafVariants.Items() {
		result = append(result, fmt.Sprintf("%s -> %s", strings.Repeat("  ", s.GetLevel()), l.String()))
	}
	return result
}

// SdcpbPath returns the sdcpb.Path, with its elements and keys based on the local schema
func (s *sharedEntryAttributes) SdcpbPath() (*sdcpb.Path, error) {
	return s.SdcpbPathInternal(s.Path())
}

// sdcpbPathInternal is the internale recursive function to calculate and the sdcpb.Path,
// with its elements and keys based on the local schema
func (s *sharedEntryAttributes) SdcpbPathInternal(spath []string) (*sdcpb.Path, error) {
	// if we moved all the way up to the root
	// we create a new sdcpb.Path and return it.
	if s.IsRoot() {
		return &sdcpb.Path{
			Elem: []*sdcpb.PathElem{},
		}, nil
	}
	// if not root, we take the parents result of this
	// recursive call and add our (s) information to the Path.
	p, err := s.parent.SdcpbPathInternal(spath)
	if err != nil {
		return nil, err
	}

	// the element has a schema attached, so we need to add a new element to the
	// path.
	if s.schema != nil {
		switch s.schema.GetSchema().(type) {
		case *sdcpb.SchemaElem_Container, *sdcpb.SchemaElem_Field, *sdcpb.SchemaElem_Leaflist:
			pe := &sdcpb.PathElem{
				Name: s.pathElemName,
				Key:  map[string]string{},
			}
			p.Elem = append(p.Elem, pe)
		}

		// the element does not have a schema attached, hence we need to add a key to
		// the last element that was pushed to the pathElems
	} else {
		name, err := s.getKeyName()
		if err != nil {
			return nil, err
		}
		p.Elem[len(p.Elem)-1].Key[name] = s.PathName()
	}
	return p, err
}

func (s *sharedEntryAttributes) TreeExport(owner string) ([]*tree_persist.TreeElement, error) {
	var lvResult []byte
	var childResults []*tree_persist.TreeElement
	var err error

	le := s.leafVariants.GetByOwner(owner)

	if le != nil && !le.Delete {
		lvResult, err = le.ValueAsBytes()
		if err != nil {
			return nil, err
		}
	}

	if len(s.GetSchemaKeys()) > 0 {
		children, err := s.FilterChilds(nil)
		if err != nil {
			return nil, err
		}
		result := []*tree_persist.TreeElement{}
		for _, c := range children {
			childexport, err := c.TreeExport(owner)
			if err != nil {
				return nil, err
			}
			if len(childexport) == 0 {
				// no childs belonging to the given owner
				continue
			}
			if len(childexport) > 1 {
				return nil, fmt.Errorf("unexpected value")
			}
			childexport[0].Name = s.pathElemName

			result = append(result, childexport...)
		}
		if len(result) > 0 {
			return result, nil
		}
	} else {
		for _, c := range s.getChildren() {
			childExport, err := c.TreeExport(owner)
			if err != nil {
				return nil, err
			}
			if len(childExport) > 0 {
				childResults = append(childResults, childExport...)
			}

		}
		if lvResult != nil || len(childResults) > 0 {
			return []*tree_persist.TreeElement{
				{
					Name:        s.pathElemName,
					Childs:      childResults,
					LeafVariant: lvResult,
				},
			}, nil
		}
	}

	return nil, nil
}

// getKeyName checks if s is a key level element in the tree, if not an error is throw
// if it is a key level element, the name of the key is determined via the ancestor schemas
func (s *sharedEntryAttributes) getKeyName() (string, error) {
	// if the entry has a schema, it cannot be a Key attribute
	if s.schema != nil {
		return "", fmt.Errorf("error %s is a schema element, can only get KeyNames for key element", strings.Join(s.Path(), " "))
	}

	// get ancestor schema
	ancestorWithSchema, levelUp := s.GetFirstAncestorWithSchema()

	// only Containers have keys, so check for that
	schemaKeys := ancestorWithSchema.GetSchemaKeys()
	if len(schemaKeys) == 0 {
		// we probably called the function on a LeafList or LeafEntry which is not a valid call to be made.
		return "", fmt.Errorf("error LeafList and Field should not have keys %s", strings.Join(s.Path(), " "))
	}

	slices.Sort(schemaKeys)
	return schemaKeys[levelUp-1], nil
}

func (s *sharedEntryAttributes) getOrCreateChilds(ctx context.Context, path types.PathSlice) (Entry, error) {
	var err error
	if len(path) == 0 {
		return s, nil
	}

	e, exists := s.childs.GetEntry(path[0])
	if !exists {
		e, err = newEntry(ctx, s, path[0], s.treeContext)
		if err != nil {
			return nil, err
		}
	}

	return e.getOrCreateChilds(ctx, path[1:])
}

// AddUpdateRecursive recursively adds the given cache.Update to the tree. Thereby creating all the entries along the path.
// if the entries along th path already exist, the existing entries are called to add the Update.
func (s *sharedEntryAttributes) AddUpdateRecursive(ctx context.Context, u *types.Update, flags *types.UpdateInsertFlags) (Entry, error) {
	idx := s.GetLevel()
	var err error
	// make sure all the keys are also present as leafs
	err = s.checkAndCreateKeysAsLeafs(ctx, u.Owner(), u.Priority(), flags)
	if err != nil {
		return nil, err
	}

	// end of path reached, add LeafEntry
	// continue with recursive add otherwise
	if idx == len(u.GetPathSlice()) {
		// delegate update handling to leafVariants
		s.leafVariants.Add(NewLeafEntry(u, flags, s))
		return s, nil
	}

	var e Entry

	var exists bool
	// if child does not exist, create Entry
	if e, exists = s.childs.GetEntry(u.GetPathSlice()[idx]); !exists {
		e, err = newEntry(ctx, s, u.GetPathSlice()[idx], s.treeContext)
		if err != nil {
			return nil, err
		}

	}
	return e.AddUpdateRecursive(ctx, u, flags)
}

// containsOnlyDefaults checks for presence containers, if only default values are present,
// such that the Entry should also be treated as a presence container
func (s *sharedEntryAttributes) containsOnlyDefaults() bool {
	// if no schema is present, we must be in a key level
	if s.schema == nil {
		return false
	}
	contSchema := s.schema.GetContainer()
	if contSchema == nil {
		return false
	}

	// only if length of childs is (more) compared to the number of
	// attributes carrying defaults, the presence condition can be met
	if s.childs.Length() > len(contSchema.ChildsWithDefaults) {
		return false
	}
	for k, v := range s.childs.GetAll() {
		// check if child name is part of ChildsWithDefaults
		if !slices.Contains(contSchema.ChildsWithDefaults, k) {
			return false
		}
		// check if the value is the default value
		le, err := v.getHighestPrecedenceLeafValue(context.TODO())
		if err != nil {
			return false
		}
		// if the owner is not Default return false
		if le.Owner() != DefaultsIntentName {
			return false
		}
	}

	return true
}

func (s *sharedEntryAttributes) GetLeafVariantEntries() LeafVariantEntries {
	return s.leafVariants
}
