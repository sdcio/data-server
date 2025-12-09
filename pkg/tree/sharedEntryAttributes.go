package tree

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
)

var (
	ValidationError = errors.New("validation error")
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
	pathCache         *sdcpb.Path
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
			path := s.SdcpbPath().CopyPathAddElem(sdcpb.NewPathElem(childname, nil))
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
				newPath := s.SdcpbPath().CopyPathAddElem(sdcpb.NewPathElem(childname, nil))
				_, err := s.tryLoadingDefault(ctx, newPath)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *sharedEntryAttributes) GetDeviations(ctx context.Context, ch chan<- *types.DeviationEntry, activeCase bool) {
	evalLeafvariants := true
	// if s is a presence container but has active childs, it should not be treated as a presence
	// container, hence the leafvariants should not be processed. For presence container with
	// childs the TypedValue.empty_val in the presence container is irrelevant.
	if s.schema.GetContainer().GetIsPresence() && len(s.GetChilds(DescendMethodActiveChilds)) > 0 {
		evalLeafvariants = false
	}

	if evalLeafvariants {
		// calculate Deviation on the LeafVariants
		s.leafVariants.GetDeviations(ctx, ch, activeCase)
	}

	// get all active childs
	activeChilds := s.GetChilds(DescendMethodActiveChilds)

	// iterate through all childs
	for cName, c := range s.getChildren() {
		// check if c is a active child (choice / case)
		_, isActiveChild := activeChilds[cName]
		// recurse the call
		c.GetDeviations(ctx, ch, isActiveChild)
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
		keySorted := make([]*sdcpb.LeafSchema, 0, len(ancestor.GetSchema().GetContainer().Keys))
		// add key leafschemas to slice
		keySorted = append(keySorted, ancestor.GetSchema().GetContainer().Keys...)
		// sort keySorted slice
		slices.SortFunc(keySorted, func(a, b *sdcpb.LeafSchema) int {
			return cmp.Compare(b.Name, a.Name)
		})

		// iterate through the keys
		var item Entry = s

		// construct the key path
		// doing so outside the loop to reuse
		path := &sdcpb.Path{
			IsRootBased: false,
		}

		for _, k := range keySorted {
			child, entryExists := s.childs.GetEntry(k.Name)
			// if the key Leaf exists continue with next key
			if entryExists {
				// if it exists, we need to check that the entry for the owner exists.
				var result []*LeafEntry
				lvs := child.GetByOwner(intentName, result)
				if len(lvs) > 0 {
					lvs[0].RemoveDeleteFlag()
					// continue with parent Entry BEFORE continuing the loop
					item = item.GetParent()
					continue
				}
			}

			// convert the key value to the schema defined Typed_Value
			tv, err := sdcpb.TVFromString(k.GetType(), item.PathName(), 0)
			if err != nil {
				return err
			}
			if !entryExists {
				// create a new entry
				child, err = newEntry(ctx, s, k.Name, s.treeContext)

				if err != nil {
					return err
				}
			}
			// add the new child entry to s
			err = s.addChild(ctx, child)
			if err != nil {
				return err
			}

			// Add the update to the tree
			_, err = child.AddUpdateRecursive(ctx, path, types.NewUpdate(nil, tv, prio, intentName, 0), insertFlag)
			if err != nil {
				return err
			}

			// continue with parent Entry
			item = item.GetParent()
		}
	}
	return nil
}

func (s *sharedEntryAttributes) populateSchema(ctx context.Context) error {
	getSchema := true
	var path *sdcpb.Path
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
				return nil
			}
		}
		path = ancesterschema.SdcpbPath().CopyPathAddElem(sdcpb.NewPathElem(s.pathElemName, nil))
	}

	if getSchema {
		// trieve if the getSchema var is still true
		schemaResp, err := s.treeContext.schemaClient.GetSchemaSdcpbPath(ctx, path)
		if err != nil {
			return err
		}
		s.schemaMutex.Lock()
		defer s.schemaMutex.Unlock()
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
		return nil, fmt.Errorf("error GetListChilds() non schema level %s", s.SdcpbPath().ToXPath(false))
	}
	if s.schema.GetContainer() == nil {
		return nil, fmt.Errorf("error GetListChilds() not a Container %s", s.SdcpbPath().ToXPath(false))
	}
	keys := s.schema.GetContainer().GetKeys()
	if len(keys) == 0 {
		return nil, fmt.Errorf("error GetListChilds() not a List Container %s", s.SdcpbPath().ToXPath(false))
	}
	actualEntries := []Entry{s}
	var newEntries []Entry

	for level := 0; level < len(keys); level++ {
		for _, e := range actualEntries {
			// add all children
			for _, c := range e.GetChilds(DescendMethodAll) {
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
		return nil, fmt.Errorf("error non schema level %s", s.SdcpbPath().ToXPath(false))
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
				childs := entry.GetChilds(DescendMethodAll)
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
				childs := entry.GetChilds(DescendMethodAll)
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
	for _, c := range s.GetChilds(v.DescendMethod()) {
		err := c.Walk(ctx, v)
		if err != nil {
			return err
		}
	}
	v.Up()
	return nil
}

func (s *sharedEntryAttributes) HoldsLeafvariants() bool {
	switch x := s.schema.GetSchema().(type) {
	case *sdcpb.SchemaElem_Container:
		return x.Container.GetIsPresence()
	case *sdcpb.SchemaElem_Leaflist:
		return true
	case *sdcpb.SchemaElem_Field:
		return true
	}
	return false
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
	for _, c := range s.GetChilds(DescendMethodActiveChilds) {
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

	activeChilds := s.GetChilds(DescendMethodActiveChilds)
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
	for _, c := range s.GetChilds(DescendMethodActiveChilds) {
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
		deletes = append(deletes, types.NewDeleteEntryImpl(s.SdcpbPath().CopyPathAddElem(sdcpb.NewPathElem(elem, nil))))
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

// PathName returns the name of the Entry
func (s *sharedEntryAttributes) PathName() string {
	return s.pathElemName
}

// String returns a string representation of the Entry
func (s *sharedEntryAttributes) String() string {
	return s.SdcpbPath().ToXPath(false)
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
	s.childs.Add(e)
	return nil
}

func (s *sharedEntryAttributes) NavigateSdcpbPath(ctx context.Context, path *sdcpb.Path) (Entry, error) {
	pathElems := path.GetElem()
	var err error
	if len(pathElems) == 0 {
		return s, nil
	}

	if path.IsRootBased {
		return s.GetRoot().NavigateSdcpbPath(ctx, path.DeepCopy().SetIsRootBased(false))
	}

	switch pathElems[0].Name {
	case ".":
		s.NavigateSdcpbPath(ctx, path.CopyAndRemoveFirstPathElem())
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
		return entry.NavigateSdcpbPath(ctx, path.CopyAndRemoveFirstPathElem())
	default:
		e, exists := s.GetChilds(DescendMethodActiveChilds)[pathElems[0].Name]
		if !exists {
			pth := &sdcpb.Path{Elem: pathElems}
			e, err = s.tryLoadingDefault(ctx, pth)
			if err != nil {
				pathStr := pth.ToXPath(false)
				return nil, fmt.Errorf("navigating tree, reached %v but child %v does not exist, trying to load defaults yielded %v", s.SdcpbPath().ToXPath(false), pathStr, err)
			}
			return e, nil
		}

		for v := range pathElems[0].PathElemNamesKeysOnly() {
			// make sure to only skip the first element
			e, err = e.NavigateSdcpbPath(ctx, &sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem(v, nil)}})
			if err != nil {
				return nil, err
			}
		}

		return e.NavigateSdcpbPath(ctx, path.CopyAndRemoveFirstPathElem())
	}

	return nil, fmt.Errorf("navigating tree, reached %v but child %v does not exist", s.SdcpbPath().ToXPath(false), pathElems)
}

func (s *sharedEntryAttributes) tryLoadingDefault(ctx context.Context, path *sdcpb.Path) (Entry, error) {
	schema, err := s.treeContext.schemaClient.GetSchemaSdcpbPath(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("error trying to load defaults for %s: %v", path.ToXPath(false), err)
	}

	upd, err := DefaultValueRetrieve(ctx, schema.GetSchema(), path)
	if err != nil {
		return nil, err
	}

	flags := types.NewUpdateInsertFlags()

	result, err := s.AddUpdateRecursive(ctx, path, upd, flags)
	if err != nil {
		return nil, fmt.Errorf("failed adding default value for %s to tree; %v", path.ToXPath(false), err)
	}

	return result, nil
}

func (s *sharedEntryAttributes) DeleteBranch(ctx context.Context, path *sdcpb.Path, owner string) error {
	var entry Entry
	var err error

	if path == nil {
		return s.deleteBranchInternal(ctx, owner)
	}

	// if the relativePath is present, we need to naviagate
	entry, err = s.NavigateSdcpbPath(ctx, path)
	if err != nil {
		return err
	}
	err = entry.DeleteBranch(ctx, nil, owner)
	if err != nil {
		return err
	}

	if entry == nil {
		return nil
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
		if entry == nil {
			// we made it all the way up to the root. So we have to return.
			return nil
		}
		// calling DeleteSubtree with the empty string, because it should not delete the owner from the higher level keys,
		// but what it will also do is delete possibly dangling key elements in the tree
		entry.deleteCanDeleteChilds(true)
	}

	return nil
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
func (s *sharedEntryAttributes) GetHighestPrecedence(result LeafVariantSlice, onlyNewOrUpdated bool, includeDefaults bool, includeExplicitDelete bool) LeafVariantSlice {
	// get the highes precedence LeafeVariant and add it to the list
	lv := s.leafVariants.GetHighestPrecedence(onlyNewOrUpdated, includeDefaults, includeExplicitDelete)
	if lv != nil {
		result = append(result, lv)
	}

	// continue with childs. Childs are part of choices, process only the "active" (highes precedence) childs
	for _, c := range s.GetChilds(DescendMethodActiveChilds) {
		result = c.GetHighestPrecedence(result, onlyNewOrUpdated, includeDefaults, includeExplicitDelete)
	}
	return result
}

func (s *sharedEntryAttributes) getHighestPrecedenceLeafValue(ctx context.Context) (*LeafEntry, error) {
	for _, x := range []string{"existing", "default"} {
		lv := s.leafVariants.GetHighestPrecedence(false, true, false)
		if lv != nil {
			return lv, nil
		}
		if x != "default" {
			_, err := s.tryLoadingDefault(ctx, s.SdcpbPath())
			if err != nil {
				return nil, err
			}
		}
	}
	return nil, fmt.Errorf("error no value present for %s", s.SdcpbPath().ToXPath(false))
}

func (s *sharedEntryAttributes) GetRootBasedEntryChain() []Entry {
	// Build the chain from root to this entry without recursion
	var chain []Entry
	var current Entry = s
	for current != nil && !current.IsRoot() {
		chain = append(chain, current)
		current = current.GetParent()
	}
	// Add the root if needed
	if current != nil {
		chain = append(chain, current)
	}
	// reverse the slice to get root-based order
	slices.Reverse(chain)
	return chain
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
func (s *sharedEntryAttributes) ValidateLevel(ctx context.Context, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats, vCfg *config.Validation) {
	// validate the mandatory statement on this entry
	if s.remainsToExist() {
		// TODO: Validate Enums
		if !vCfg.DisabledValidators.Mandatory {
			s.validateMandatory(ctx, resultChan, stats)
		}
		if !vCfg.DisabledValidators.Leafref {
			s.validateLeafRefs(ctx, resultChan, stats)
		}
		if !vCfg.DisabledValidators.LeafrefMinMaxAttributes {
			s.validateLeafListMinMaxAttributes(resultChan, stats)
		}
		if !vCfg.DisabledValidators.Pattern {
			s.validatePattern(resultChan, stats)
		}
		if !vCfg.DisabledValidators.MustStatement {
			s.validateMustStatements(ctx, resultChan, stats)
		}
		if !vCfg.DisabledValidators.Length {
			s.validateLength(resultChan, stats)
		}
		if !vCfg.DisabledValidators.Range {
			s.validateRange(resultChan, stats)
		}
		if !vCfg.DisabledValidators.MaxElements {
			s.validateMinMaxElements(resultChan, stats)
		}
	}
}

// validateRange int and uint types (Leaf and Leaflist) define ranges which configured values must lay in.
// validateRange does check this condition.
func (s *sharedEntryAttributes) validateRange(resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {

	// if no schema present or Field and LeafList Types do not contain any ranges, return there is nothing to check
	if s.GetSchema() == nil || (len(s.GetSchema().GetField().GetType().GetRange()) == 0 && len(s.GetSchema().GetLeaflist().GetType().GetRange()) == 0) {
		return
	}

	lv := s.leafVariants.GetHighestPrecedence(false, true, false)
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

	// range through the tvs and check that they are in range
	for _, tv := range tvs {
		// we need to distinguish between unsigned and singned ints
		switch typeSchema.TypeName {
		case "uint8", "uint16", "uint32", "uint64":
			// procede with the unsigned ints
			urnges := utils.NewUrnges()
			// add the defined ranges to the ranges struct
			for _, r := range typeSchema.GetRange() {
				urnges.AddRange(r.Min.Value, r.Max.Value)
			}
			stats.Add(types.StatTypeRange, uint32(len(typeSchema.GetRange())))

			// check the value lays within any of the ranges
			if !urnges.IsWithinAnyRange(tv.GetUintVal()) {
				resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("path %s, value %d not within any of the expected ranges %s", s.SdcpbPath().ToXPath(false), tv.GetUintVal(), urnges.String()), types.ValidationResultEntryTypeError)
			}

		case "int8", "int16", "int32", "int64":
			// procede with the signed ints
			srnges := utils.NewSrnges()
			for _, r := range typeSchema.GetRange() {
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
			stats.Add(types.StatTypeRange, uint32(len(typeSchema.GetRange())))
			// check the value lays within any of the ranges
			if !srnges.IsWithinAnyRange(tv.GetIntVal()) {
				resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("path %s, value %d not within any of the expected ranges %s", s.SdcpbPath().ToXPath(false), tv.GetIntVal(), srnges.String()), types.ValidationResultEntryTypeError)
			}
		}
	}
}

func (s *sharedEntryAttributes) validateMinMaxElements(resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	var contSchema *sdcpb.ContainerSchema
	if contSchema = s.GetSchema().GetContainer(); contSchema == nil {
		// if it is not a container, return
		return
	}
	if len(contSchema.GetKeys()) == 0 {
		// if it is not a list, return
		return
	}

	// get all the childs, skipping the key levels
	childs, err := s.GetListChilds()
	if err != nil {
		resultChan <- types.NewValidationResultEntry("unknown", fmt.Errorf("error getting childs for min/max-elements check %v", err), types.ValidationResultEntryTypeError)
	}

	intMin := int(contSchema.GetMinElements())
	intMax := int(contSchema.GetMaxElements())

	// early exit if no specific min/max defined
	if intMin <= 0 && intMax <= 0 {
		return
	}

	// define function to figure out associated owners / intents

	ownersSet := map[string]struct{}{}
	for _, child := range childs {
		childAttributes := child.GetChilds(DescendMethodActiveChilds)
		keyName := contSchema.GetKeys()[0].GetName()
		if keyAttr, ok := childAttributes[keyName]; ok {
			highestPrec := keyAttr.GetHighestPrecedence(nil, false, false, false)
			if len(highestPrec) > 0 {
				owner := highestPrec[0].Update.Owner()
				ownersSet[owner] = struct{}{}
			}
		}
	}
	// dedup the owners
	owners := []string{}
	for k := range ownersSet {
		owners = append(owners, k)
	}

	if len(childs) < intMin {
		for _, owner := range owners {
			resultChan <- types.NewValidationResultEntry(owner, fmt.Errorf("Min-Elements violation on %s expected %d actual %d", s.SdcpbPath().ToXPath(false), intMin, len(childs)), types.ValidationResultEntryTypeError)
		}
	}
	if intMax > 0 && len(childs) > intMax {
		for _, owner := range owners {
			resultChan <- types.NewValidationResultEntry(owner, fmt.Errorf("Max-Elements violation on %s expected %d actual %d", s.SdcpbPath().ToXPath(false), intMax, len(childs)), types.ValidationResultEntryTypeError)
		}
	}
	stats.Add(types.StatTypeMinMaxElementsList, 1)
}

// validateLeafListMinMaxAttributes validates the Min-, and Max-Elements attribute of the Entry if it is a Leaflists.
func (s *sharedEntryAttributes) validateLeafListMinMaxAttributes(resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	if schema := s.schema.GetLeaflist(); schema != nil {
		if schema.GetMinElements() > 0 || schema.GetMaxElements() < math.MaxUint64 {
			if lv := s.leafVariants.GetHighestPrecedence(false, true, false); lv != nil {
				tv := lv.Update.Value()

				if val := tv.GetLeaflistVal(); val != nil {
					// check minelements if set
					if schema.GetMinElements() > 0 && len(val.GetElement()) < int(schema.GetMinElements()) {
						resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("leaflist %s defines %d min-elements but only %d elements are present", s.SdcpbPath().ToXPath(false), schema.MinElements, len(val.GetElement())), types.ValidationResultEntryTypeError)
					}
					// check maxelements if set
					if uint64(len(val.GetElement())) > uint64(schema.GetMaxElements()) {
						resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("leaflist %s defines %d max-elements but %d elements are present", s.SdcpbPath().ToXPath(false), schema.GetMaxElements(), len(val.GetElement())), types.ValidationResultEntryTypeError)
					}
				}
				stats.Add(types.StatTypeMinMaxElementsLeaflist, 1)
			}
		}
	}
}

func (s *sharedEntryAttributes) validateLength(resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	if schema := s.schema.GetField(); schema != nil {

		if len(schema.GetType().GetLength()) == 0 {
			return
		}

		lv := s.leafVariants.GetHighestPrecedence(false, true, false)
		if lv == nil {
			return
		}
		value := lv.Value().GetStringVal()
		actualLength := utf8.RuneCountInString(value)

		for _, lengthDef := range schema.GetType().GetLength() {

			if lengthDef.Min.Value <= uint64(actualLength) && uint64(actualLength) <= lengthDef.Max.Value {
				// continue if the length is within the range
				continue
			}
			// this is already the failure case
			lenghts := []string{}
			for _, lengthDef := range schema.GetType().GetLength() {
				lenghts = append(lenghts, fmt.Sprintf("%d..%d", lengthDef.Min.Value, lengthDef.Max.Value))
			}
			resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("error length of Path: %s, Value: %s not within allowed length %s", s.SdcpbPath().ToXPath(false), value, strings.Join(lenghts, ", ")), types.ValidationResultEntryTypeError)
		}
		stats.Add(types.StatTypeLength, uint32(len(schema.GetType().GetLength())))
	}
}

func (s *sharedEntryAttributes) validatePattern(resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	if schema := s.schema.GetField(); schema != nil {
		if len(schema.Type.Patterns) == 0 {
			return
		}

		lv := s.leafVariants.GetHighestPrecedence(false, true, false)
		if lv == nil {
			return
		}
		value := lv.Value().GetStringVal()
		for _, pattern := range schema.GetType().GetPatterns() {
			if p := pattern.GetPattern(); p != "" {
				matched, err := regexp.MatchString(p, value)
				if err != nil {
					resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("failed compiling regex %s defined for %s", p, s.SdcpbPath().ToXPath(false)), types.ValidationResultEntryTypeError)
					continue
				}
				if (!matched && !pattern.Inverted) || (pattern.GetInverted() && matched) {
					resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("value %s of %s does not match regex %s (inverted: %t)", value, s.SdcpbPath().ToXPath(false), p, pattern.GetInverted()), types.ValidationResultEntryTypeError)
				}
			}

		}
		stats.Add(types.StatTypePattern, uint32(len(schema.GetType().GetPatterns())))
	}
}

// validateMandatory validates that all the mandatory attributes,
// defined by the schema are present either in the tree or in the index.
func (s *sharedEntryAttributes) validateMandatory(ctx context.Context, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	log := logf.FromContext(ctx)
	if !s.remainsToExist() {
		return
	}
	if s.schema != nil {
		switch s.schema.GetSchema().(type) {
		case *sdcpb.SchemaElem_Container:
			for _, c := range s.schema.GetContainer().GetMandatoryChildrenConfig() {
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
					log.Error(ValidationError, "mandatory attribute could not be found as child, field or choice", "path", s.SdcpbPath().ToXPath(false), "attribute", c.Name)
				}

				s.validateMandatoryWithKeys(ctx, len(s.GetSchema().GetContainer().GetKeys()), attributes, choiceName, resultChan)
			}
			stats.Add(types.StatTypeMandatory, uint32(len(s.schema.GetContainer().GetMandatoryChildrenConfig())))
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
			v, existsInTree = s.GetChilds(DescendMethodActiveChilds)[attr]
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
				resultChan <- types.NewValidationResultEntry("unknown", fmt.Errorf("error mandatory child %s does not exist, path: %s", attributes, s.SdcpbPath().ToXPath(false)), types.ValidationResultEntryTypeError)
				return
			}
			// if it is a mandatory choice
			resultChan <- types.NewValidationResultEntry("unknown", fmt.Errorf("error mandatory choice %s [attributes: %s] does not exist, path: %s", choiceName, attributes, s.SdcpbPath().ToXPath(false)), types.ValidationResultEntryTypeError)
			return
		}
		return
	}

	for _, c := range s.GetChilds(DescendMethodActiveChilds) {
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
	for _, child := range s.GetChilds(DescendMethodActiveChilds) {
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

func (s *sharedEntryAttributes) GetChild(name string) (Entry, bool) {
	return s.childs.GetEntry(name)
}

func (s *sharedEntryAttributes) GetChilds(d DescendMethod) EntryMap {
	if s.schema == nil {
		return s.childs.GetAll()
	}

	switch d {
	case DescendMethodAll:
		return s.childs.GetAll()
	case DescendMethodActiveChilds:
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
	return nil
}

// // filterActiveChoiceCaseChilds returns the list of child elements. In case the Entry is
// // a container with a / multiple choices, the list of childs is filtered to only return the
// // cases that have the highest precedence.
// func (s *sharedEntryAttributes) filterActiveChoiceCaseChilds() map[string]Entry {
// 	if s.schema == nil {
// 		return s.childs.GetAll()
// 	}

// 	skipAttributesList := s.choicesResolvers.GetSkipElements()
// 	// if there are no items that should be skipped, take a shortcut
// 	// and simply return all childs straight away
// 	if len(skipAttributesList) == 0 {
// 		return s.childs.GetAll()
// 	}
// 	result := map[string]Entry{}
// 	// optimization option: sort the slices and forward in parallel, lifts extra burden that the contains call holds.
// 	for childName, child := range s.childs.GetAll() {
// 		if slices.Contains(skipAttributesList, childName) {
// 			continue
// 		}
// 		result[childName] = child
// 	}
// 	return result
// }

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
func (s *sharedEntryAttributes) SdcpbPath() *sdcpb.Path {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	if s.pathCache != nil {
		return s.pathCache
	}
	if s.IsRoot() {
		path := &sdcpb.Path{
			IsRootBased: true,
		}
		// populate cache
		s.pathCache = path
		return path
	}

	var path *sdcpb.Path
	if s.schema == nil {
		path = s.parent.SdcpbPath().DeepCopy()
		parentSchema, levelsUp := s.GetFirstAncestorWithSchema()
		schemaKeys := parentSchema.GetSchemaKeys()
		slices.Sort(schemaKeys)
		keyName := schemaKeys[levelsUp-1]
		path.GetElem()[len(path.GetElem())-1].Key[keyName] = s.pathElemName
	} else {
		path = s.parent.SdcpbPath().CopyPathAddElem(sdcpb.NewPathElem(s.pathElemName, map[string]string{}))
	}
	// populate cache
	s.pathCache = path
	return path
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

func (s *sharedEntryAttributes) getOrCreateChilds(ctx context.Context, path *sdcpb.Path) (Entry, error) {
	if path == nil || len(path.Elem) == 0 {
		return s, nil
	}

	var current Entry = s
	for i, pe := range path.Elem {
		// Step 1: Find or create the child for the path element name
		newCurrent, exists := current.GetChilds(DescendMethodAll)[pe.Name]
		if !exists {
			var err error
			child, err := newEntry(ctx, current, pe.Name, s.treeContext)
			if err != nil {
				return nil, err
			}
			if err := current.addChild(ctx, child); err != nil {
				return nil, err
			}
			newCurrent = child
		}
		current = newCurrent

		// sort keys
		keys := make([]string, 0, len(pe.Key))
		for key := range pe.Key {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		// Step 2: For each key, find or create the key child
		for _, key := range keys {
			newCurrent, exists = current.GetChilds(DescendMethodAll)[pe.Key[key]]
			if !exists {
				var err error
				keyChild, err := newEntry(ctx, current, pe.Key[key], s.treeContext)
				if err != nil {
					return nil, err
				}
				if err := current.addChild(ctx, keyChild); err != nil {
					return nil, err
				}
				newCurrent = keyChild
			}
			current = newCurrent
		}

		// If this is the last PathElem, return the current node
		if i == len(path.Elem)-1 {
			return current, nil
		}
	}

	return current, nil
}

// AddUpdateRecursive recursively adds the given cache.Update to the tree. Thereby creating all the entries along the path.
// if the entries along th path already exist, the existing entries are called to add the Update.
func (s *sharedEntryAttributes) AddUpdateRecursive(ctx context.Context, path *sdcpb.Path, u *types.Update, flags *types.UpdateInsertFlags) (Entry, error) {
	var err error
	relPath := path

	if path.IsRootBased && !s.IsRoot() {
		// calculate the relative path for the add
		relPath, err = path.AbsToRelativePath(s.SdcpbPath())
		if err != nil {
			return nil, err
		}
	}
	return s.addUpdateRecursiveInternal(ctx, relPath, 0, u, flags)
}
func (s *sharedEntryAttributes) addUpdateRecursiveInternal(ctx context.Context, path *sdcpb.Path, idx int, u *types.Update, flags *types.UpdateInsertFlags) (Entry, error) {

	// make sure all the keys are also present as leafs
	err := s.checkAndCreateKeysAsLeafs(ctx, u.Owner(), u.Priority(), flags)
	if err != nil {
		return nil, err
	}
	// end of path reached, add LeafEntry
	// continue with recursive add otherwise
	if path == nil || len(path.GetElem()) == 0 || idx >= len(path.GetElem()) {
		// delegate update handling to leafVariants
		s.leafVariants.Add(NewLeafEntry(u, flags, s))
		return s, nil
	}

	var e Entry
	var x Entry = s
	var exists bool
	for name := range path.GetElem()[idx].PathElemNames() {
		if e, exists = x.GetChilds(DescendMethodAll)[name]; !exists {
			newE, err := newEntry(ctx, x, name, s.treeContext)
			if err != nil {
				return nil, err
			}
			err = x.addChild(ctx, newE)
			if err != nil {
				return nil, err
			}
			e = newE
		}
		x = e
	}

	return x.addUpdateRecursiveInternal(ctx, path, idx+1, u, flags)
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
