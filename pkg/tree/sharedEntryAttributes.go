package tree

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// sharedEntryAttributes contains the attributes shared by Entry and RootEntry
type sharedEntryAttributes struct {
	// parent entry, nil for the root Entry
	parent api.Entry
	// pathElemName the path elements name the entry represents
	pathElemName string
	// childs mutual exclusive with LeafVariants
	childs *api.ChildMap
	// leafVariants mutual exclusive with Childs
	// If Entry is a leaf it can hold multiple leafVariants
	leafVariants *api.LeafVariants
	// schema the schema element for this entry
	schema      *sdcpb.SchemaElem
	schemaMutex sync.RWMutex

	choicesResolvers api.ChoiceResolvers

	treeContext api.TreeContext

	// state cache
	cacheMutex        sync.Mutex
	cacheShouldDelete *bool
	cacheCanDelete    *bool
	cacheRemains      *bool
	pathCache         *sdcpb.Path
	level             *int
}

// NewEntry constructor for Entries
func NewEntry(ctx context.Context, parent api.Entry, pathElemName string, tc api.TreeContext) (*sharedEntryAttributes, error) {
	// create a new sharedEntryAttributes instance
	sea, err := NewSharedEntryAttributes(ctx, parent, pathElemName, tc)
	if err != nil {
		return nil, err
	}

	// add the Entry as a child to the parent Entry
	err = parent.AddChild(ctx, sea)
	return sea, err
}

func (s *sharedEntryAttributes) DeepCopy(tc api.TreeContext, parent api.Entry) (api.Entry, error) {
	result := &sharedEntryAttributes{
		parent:           parent,
		pathElemName:     s.pathElemName,
		childs:           api.NewChildMap(),
		schema:           s.schema,
		treeContext:      tc,
		choicesResolvers: s.choicesResolvers.DeepCopy(),
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

func (s *sharedEntryAttributes) GetChildMap() *api.ChildMap {
	return s.childs
}

func NewSharedEntryAttributes(ctx context.Context, parent api.Entry, pathElemName string, tc api.TreeContext) (*sharedEntryAttributes, error) {
	s := &sharedEntryAttributes{
		parent:       parent,
		pathElemName: pathElemName,
		childs:       api.NewChildMap(),
		treeContext:  tc,
	}
	s.leafVariants = api.NewLeafVariants(tc, s)

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

func (s *sharedEntryAttributes) GetTreeContext() api.TreeContext {
	return s.treeContext
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
		ancestor, levelsUp := ops.GetFirstAncestorWithSchema(s)

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
		ancesterschema, levelUp := ops.GetFirstAncestorWithSchema(s)

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
		schemaResp, err := s.treeContext.SchemaClient().GetSchemaSdcpbPath(ctx, path)
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

// GetParent returns the parent entry
func (s *sharedEntryAttributes) GetParent() api.Entry {
	return s.parent
}

// IsRoot returns true if the element has no parent elements, hence is the root of the tree
func (s *sharedEntryAttributes) IsRoot() bool {
	return s.parent == nil
}

// GetLevel returns the level / depth position of this element in the tree
func (s *sharedEntryAttributes) GetLevel() int {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	// if level is cached, return level
	if s.level != nil {
		return *s.level
	}
	// if we're at the root level, return 0
	if s.IsRoot() {
		return 0
	}
	// Count levels iteratively by walking up to root
	level := 0
	current := s.parent
	for current != nil {
		level++
		current = current.GetParent()
	}
	// cache level value
	s.level = &level
	return level
}

// canDelete checks if the entry can be Deleted.
// This is e.g. to cover e.g. defaults and running. They can be deleted, but should not, they are basically implicitly existing.
// In caomparison to
//   - remainsToExists() returns true, because they remain to exist even though implicitly.
//   - shouldDelete() returns false, because no explicit delete should be issued for them.
func (s *sharedEntryAttributes) CanDelete() bool {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	if s.cacheCanDelete != nil {
		return *s.cacheCanDelete
	}

	leafVariantCanDelete := s.leafVariants.CanDelete()
	if !leafVariantCanDelete {
		s.cacheCanDelete = utils.BoolPtr(false)
		return *s.cacheCanDelete
	}

	// handle containers
	for _, c := range s.GetChilds(types.DescendMethodActiveChilds) {
		canDelete := c.CanDelete()
		if !canDelete {
			s.cacheCanDelete = utils.BoolPtr(false)
			return *s.cacheCanDelete
		}
	}
	s.cacheCanDelete = utils.BoolPtr(true)
	return *s.cacheCanDelete
}

func (s *sharedEntryAttributes) CanDeleteBranch(keepDefault bool) bool {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	leafVariantCanDelete := s.leafVariants.CanDeleteBranch(keepDefault)
	if !leafVariantCanDelete {
		return false
	}

	// handle containers
	for _, c := range s.childs.GetAll() {
		canDelete := c.CanDeleteBranch(keepDefault)
		if !canDelete {
			return false
		}
	}

	return true
}

// shouldDelete checks if a container or Leaf(List) is to be explicitly deleted.
func (s *sharedEntryAttributes) ShouldDelete() bool {
	// see if we have the value cached
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	if s.cacheShouldDelete != nil {
		return *s.cacheShouldDelete
	}
	// check if the leafVariants result in a should delete
	leafVariantshouldDelete := s.leafVariants.ShouldDelete()

	// for containers, it is basically a canDelete() check.
	canDelete := false
	// but a real delete should only be added if there is at least one shouldDelete() == true
	shouldDelete := false

	activeChilds := s.GetChilds(types.DescendMethodActiveChilds)
	// if we have no active childs, we can and should delete.
	if len(s.choicesResolvers) > 0 && len(activeChilds) == 0 {
		canDelete = true
		shouldDelete = true
	}

	// iterate through the active childs
	for _, c := range activeChilds {
		// check if the child can be deleted
		canDelete = c.CanDelete()
		// if it can explicitly not be deleted, then the result is clear, we should not delete
		if !canDelete {
			break
		}
		// if it can be deleted we need to check if there is a contibuting entry that
		// requires deletion only if there is a contributing shouldDelete() == true then we must issue
		// a real delete
		shouldDelete = shouldDelete || c.ShouldDelete()
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
	result := leafVariantshouldDelete || (canDelete && shouldDelete && s.leafVariants.CanDelete())

	s.cacheShouldDelete = &result
	return result
}

func (s *sharedEntryAttributes) RemainsToExist() bool {
	// see if we have the value cached
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	if s.cacheRemains != nil {
		return *s.cacheRemains
	}
	leafVariantResult := s.leafVariants.RemainsToExist()

	// handle containers
	childsRemain := false
	for _, c := range s.GetChilds(types.DescendMethodActiveChilds) {
		childsRemain = c.RemainsToExist()
		if childsRemain {
			break
		}
	}

	// assumption is, that if the entry exists, there is at least a running value available.
	remains := leafVariantResult || childsRemain

	s.cacheRemains = &remains
	return remains
}

// PathName returns the name of the Entry
func (s *sharedEntryAttributes) PathName() string {
	return s.pathElemName
}

// String returns a string representation of the Entry
func (s *sharedEntryAttributes) String() string {
	return s.SdcpbPath().ToXPath(false)
}

// AddChild add an entry to the list of child entries for the entry.
func (s *sharedEntryAttributes) AddChild(ctx context.Context, e api.Entry) error {
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

func (s *sharedEntryAttributes) ChoicesResolvers() api.ChoiceResolvers {
	return s.choicesResolvers
}

func (s *sharedEntryAttributes) NavigateSdcpbPath(ctx context.Context, path *sdcpb.Path) (api.Entry, error) {
	pathElems := path.GetElem()
	var err error
	if len(pathElems) == 0 {
		return s, nil
	}

	if path.IsRootBased {
		return ops.GetRoot(s).NavigateSdcpbPath(ctx, path.DeepCopy().SetIsRootBased(false))
	}

	switch pathElems[0].Name {
	case ".":
		s.NavigateSdcpbPath(ctx, path.CopyAndRemoveFirstPathElem())
	case "..":
		var entry api.Entry
		entry = s.parent
		// we need to skip key levels in the tree
		// if the next path element is again .. we need to skip key values that are present in the tree
		// If it is a sub-entry instead, we need to stay in the brach that is defined by the key values
		// hence only delegate the call to the parent

		if len(pathElems) > 1 && pathElems[1].Name == ".." {
			entry, _ = ops.GetFirstAncestorWithSchema(s)
		}
		return entry.NavigateSdcpbPath(ctx, path.CopyAndRemoveFirstPathElem())
	default:
		e, exists := s.GetChilds(types.DescendMethodActiveChilds)[pathElems[0].Name]
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

func (s *sharedEntryAttributes) tryLoadingDefault(ctx context.Context, path *sdcpb.Path) (api.Entry, error) {
	schema, err := s.treeContext.SchemaClient().GetSchemaSdcpbPath(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("error trying to load defaults for %s: %v", path.ToXPath(false), err)
	}

	upd, err := ops.DefaultValueRetrieve(ctx, schema.GetSchema(), path)
	if err != nil {
		return nil, err
	}

	flags := types.NewUpdateInsertFlags()

	result, err := ops.AddUpdateRecursive(ctx, s, path, upd, flags)
	if err != nil {
		return nil, fmt.Errorf("failed adding default value for %s to tree; %v", path.ToXPath(false), err)
	}

	return result, nil
}

func (s *sharedEntryAttributes) DeleteCanDeleteChilds(keepDefault bool) {
	// otherwise check all
	for childname, child := range s.childs.GetAll() {
		if child.CanDeleteBranch(keepDefault) {
			s.childs.DeleteChild(childname)
		}
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
	// // no choice info present
	// if ci == nil {
	// 	return
	// }
	// create a new choiceCasesResolvers struct
	choicesResolvers := api.ChoiceResolvers{}

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
	for _, child := range s.GetChilds(types.DescendMethodActiveChilds) {
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
				valWDeleted := ops.GetHighestPrecedenceValueOfBranch(child, api.HighestPrecedenceFilterAll)
				if valWDeleted <= highestWDeleted {
					highestWDeleted = valWDeleted
					if child.CanDelete() {
						isDeleted = true
					}
				}

				valWODeleted := ops.GetHighestPrecedenceValueOfBranch(child, api.HighestPrecedenceFilterWithoutDeleted)
				if valWODeleted <= highestWODeleted {
					highestWODeleted = valWODeleted
				}
				valWONew := ops.GetHighestPrecedenceValueOfBranch(child, api.HighestPrecedenceFilterWithoutNew)
				if valWONew <= highestWONew {
					highestWONew = valWONew
				}

			}
			choiceResolver.SetValue(elem, highestWODeleted, highestWDeleted, highestWONew, isDeleted)
		}
	}
	return nil
}

func (s *sharedEntryAttributes) GetChilds(d types.DescendMethod) api.EntryMap {
	if s.schema == nil {
		return s.childs.GetAll()
	}

	switch d {
	case types.DescendMethodAll:
		return s.childs.GetAll()
	case types.DescendMethodActiveChilds:
		skipAttributesList := s.choicesResolvers.GetSkipElements()
		// if there are no items that should be skipped, take a shortcut
		// and simply return all childs straight away
		if len(skipAttributesList) == 0 {
			return s.childs.GetAll()
		}
		result := map[string]api.Entry{}
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
		// For key-level entries (schema == nil), we need to add a key to the parent's last element.
		// To minimize memory allocation, we use a hybrid approach:
		// - Shallow copy earlier path elements (they're immutable once created)
		// - Deep copy only the last element (we need to modify its Key map)
		// This is more efficient than DeepCopy() which copies all elements.

		parentPath := s.parent.SdcpbPath()
		parentElems := parentPath.GetElem()

		// Allocate exact size needed (same as parent) - avoids append's growth heuristics
		newElems := make([]*sdcpb.PathElem, len(parentElems))

		// Shallow copy all path elements except the last one.
		// These elements won't be modified, so sharing pointers is safe.
		copy(newElems[:len(parentElems)-1], parentElems[:len(parentElems)-1])

		// Deep copy only the last element since we need to modify its Key map.
		// First, copy the element's Key map to avoid modifying the parent's path.
		lastElem := parentElems[len(parentElems)-1]
		keysCopy := make(map[string]string, len(lastElem.GetKey()))
		for k, v := range lastElem.GetKey() {
			keysCopy[k] = v
		}
		newElems[len(parentElems)-1] = sdcpb.NewPathElem(
			lastElem.GetName(),
			keysCopy,
		)

		// Now safely modify the copied Key map with this entry's key.
		// Since this entry has no schema (it's a key-level entry), we need to:
		// 1. Find the ancestor that has schema information
		// 2. Determine which key in that schema corresponds to this entry
		// 3. Add the key-value pair to the parent's last path element

		parentSchema, levelsUp := ops.GetFirstAncestorWithSchema(s)
		// Get the list of keys defined in the parent schema.
		schemaKeys := ops.GetSchemaKeys(parentSchema)
		// Sort keys to match the tree's insertion order (consistent with how keys are organized in levels)
		slices.Sort(schemaKeys)
		// Select the key name based on how many levels up the schema is.
		// If levelsUp=1, we're one level below the schema, so we use schemaKeys[0], etc.
		keyName := schemaKeys[levelsUp-1]
		// Set this entry's name as the value for the selected key in the parent's last element
		newElems[len(newElems)-1].Key[keyName] = s.pathElemName

		// Construct the new path with the modified elements
		path = &sdcpb.Path{
			Origin:      parentPath.Origin,
			Target:      parentPath.Target,
			IsRootBased: parentPath.IsRootBased,
			Elem:        newElems,
		}
	} else {
		// For entries with schemas, simply append a new path element to the parent's path.
		path = s.parent.SdcpbPath().CopyPathAddElem(sdcpb.NewPathElem(s.pathElemName, nil))
	}
	// populate cache
	s.pathCache = path
	return path
}

func (s *sharedEntryAttributes) GetLeafVariants() *api.LeafVariants {
	return s.leafVariants
}
