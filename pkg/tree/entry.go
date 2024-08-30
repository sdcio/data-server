package tree

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/yang-parser/xpath"
	"github.com/sdcio/yang-parser/xpath/grammars/expr"
	"google.golang.org/protobuf/proto"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

const (
	KeysIndexSep       = "_"
	DefaultValuesPrio  = int32(math.MaxInt32 - 90)
	DefaultsIntentName = "default"
	RunningValuesPrio  = int32(math.MaxInt32 - 100)
	RunningIntentName  = "running"
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
	err = parent.addChild(ctx, newEntry)
	return newEntry, err
}

// Entry is the primary Element of the Tree.
type Entry interface {
	// Path returns the Path as PathSlice
	Path() PathSlice
	// PathName returns the last Path element, the name of the Entry
	PathName() string
	// addChild Add a child entry
	addChild(context.Context, Entry) error
	// AddCacheUpdateRecursive Add the given cache.Update to the tree
	AddCacheUpdateRecursive(ctx context.Context, u *cache.Update, new bool) (Entry, error)
	// StringIndent debug tree struct as indented string slice
	StringIndent(result []string) []string
	// GetHighesPrio return the new cache.Update entried from the tree that are the highes priority.
	// If the onlyNewOrUpdated option is set to true, only the New or Updated entries will be returned
	// It will append to the given list and provide a new pointer to the slice
	GetHighestPrecedence(result LeafVariantSlice, onlyNewOrUpdated bool) LeafVariantSlice
	// GetByOwner returns the branches Updates by owner
	GetByOwner(owner string, result []*LeafEntry) []*LeafEntry
	// markOwnerDelete Sets the delete flag on all the LeafEntries belonging to the given owner.
	markOwnerDelete(o string)
	// GetDeletes returns the cache-updates that are not updated, have no lower priority value left and hence should be deleted completely
	GetDeletes(paths PathSlices, aggregatePaths bool) PathSlices
	// Walk takes the EntryVisitor and applies it to every Entry in the tree
	Walk(f EntryVisitor) error
	// Validate kicks off validation
	Validate(ctx context.Context, errchan chan<- error)
	// validateMandatory the Mandatory schema field
	validateMandatory(errchan chan<- error)
	// validateMandatoryWithKeys is an internally used function that us called by validateMandatory in case
	// the container has keys defined that need to be skipped before the mandatory attributes can be checked
	validateMandatoryWithKeys(level int, attribute string) error
	// getHighestPrecedenceValueOfBranch returns the highes Precedence Value (lowest Priority value) of the brach that starts at this Entry
	getHighestPrecedenceValueOfBranch() int32
	// GetSchema returns the *sdcpb.SchemaElem of the Entry
	GetSchema() *sdcpb.SchemaElem
	// IsRoot returns true if the Entry is the root of the tree
	IsRoot() bool
	// FinishInsertionPhase indicates, that the insertion of Entries into the tree is over
	// Hence calculations for e.g. choice/case can be performed.
	FinishInsertionPhase()
	// GetParent returns the parent entry
	GetParent() Entry
	// Navigate navigates the tree according to the given path and returns the referenced entry or nil if it does not exist.
	Navigate(ctx context.Context, path []string, isRootPath bool) (Entry, error)
	NavigateSdcpbPath(ctx context.Context, path []*sdcpb.PathElem, isRootPath bool) (Entry, error)
	// GetFirstAncestorWithSchema returns the first parent node which has a schema set.
	// if the parent has no schema (is a key element in the tree) it will recurs the call to the parents parent.
	// the level of recursion is indicated via the levelUp attribute
	GetFirstAncestorWithSchema() (ancestor Entry, levelUp int)
	// SdcpbPath returns the sdcpb.Path struct for the Entry
	SdcpbPath() (*sdcpb.Path, error)
	// SdcpbPathInternal is the internal function to calculate the SdcpbPath
	SdcpbPathInternal(spath []string) (*sdcpb.Path, error)
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
	// Or will disappear from device (running) as part of the SetIntent action.
	remainsToExist() bool
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

	// populate the schema
	err := s.populateSchema(ctx)
	if err != nil {
		return nil, err
	}

	// initialize the choice case resolvers with the schema information
	s.initChoiceCasesResolvers()

	return s, nil
}

func (s *sharedEntryAttributes) GetRoot() Entry {
	if s.parent == nil {
		return s
	}
	return s.parent.GetRoot()
}

func (s *sharedEntryAttributes) populateSchema(ctx context.Context) error {
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
		schemaResp, err := s.treeContext.treeSchemaCacheClient.GetSchema(ctx, s.Path())
		if err != nil {
			return err
		}
		s.schema = schemaResp.GetSchema()
	}
	return nil
}

// GetSchema return the schema fiels of the Entry
func (s *sharedEntryAttributes) GetSchema() *sdcpb.SchemaElem {
	return s.schema
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
func (s *sharedEntryAttributes) getAggregatedDeletes(deletes PathSlices, aggregatePaths bool) PathSlices {
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
			c, exists := s.childs[n]
			// these keys should aways exist, so for now we do not catch the non existing key case
			if exists && c.remainsToExist() {
				// if not all the keys are marked for deletion, we need to revert to regular deletion
				doAggregateDelete = false
				break
			}
		}
		// if aggregate delet is possible do it
		if doAggregateDelete {
			// by adding the key path to the deletes
			deletes = append(deletes, s.Path())
		} else {
			// otherwise continue with deletion on the childs.
			for _, c := range s.childs {
				deletes = c.GetDeletes(deletes, aggregatePaths)
			}
		}
		return deletes
	}
	return s.getRegularDeletes(deletes, aggregatePaths)
}

func (s *sharedEntryAttributes) remainsToExist() bool {

	leafVariantResult := len(s.leafVariants) > 0 && !s.leafVariants.shouldDelete()

	// handle containers
	childsRemain := false
	for _, c := range s.childs {
		childsRemain = c.remainsToExist()
		if childsRemain {
			break
		}
	}

	// assumption is, that if the entry exists, there is at least a running value available.
	return leafVariantResult || childsRemain
}

// getRegularDeletes performs deletion calculation on elements that have a schema attached.
func (s *sharedEntryAttributes) getRegularDeletes(deletes PathSlices, aggregate bool) PathSlices {
	// if entry is a container type, check the keys, to be able to
	// issue a delte for the whole branch at once via keys
	switch s.schema.GetSchema().(type) {
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
	}

	if !s.remainsToExist() && !s.IsRoot() && len(s.GetSchemaKeys()) == 0 {
		return append(deletes, s.Path())
	}

	for _, e := range s.childs {
		deletes = e.GetDeletes(deletes, aggregate)
	}
	return deletes
}

// GetDeletes calculate the deletes that need to be send to the device.
func (s *sharedEntryAttributes) GetDeletes(deletes PathSlices, aggregatePaths bool) PathSlices {

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
	if s.parent == nil {
		return nil, 0
	}
	// check if the parent has a schema
	if s.parent.GetSchema() != nil {
		// if so return it with level 1
		return s.parent, 1
	}
	// direct parent does not have a schema, recurse the call
	schema, level := s.parent.GetFirstAncestorWithSchema()
	// increate the level returned by the parent to
	// reflect this entry as a level and return
	return schema, level + 1
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
func (s *sharedEntryAttributes) Path() PathSlice {
	// special handling for root node
	if s.parent == nil {
		return PathSlice{}
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
	if len(s.leafVariants) > 0 {
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
	s.childs[e.PathName()] = e

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
		if pathElems[1].Name == ".." {
			entry, _ = s.GetFirstAncestorWithSchema()
		}
		return entry.NavigateSdcpbPath(ctx, pathElems[1:], false)
	default:
		e, exists := s.filterActiveChoiceCaseChilds()[pathElems[0].Name]
		if !exists {
			e, err = s.tryLoading(ctx, []string{pathElems[0].Name})
			if err != nil {
				return nil, err
			}
		}
		for _, v := range pathElems[0].Key {
			e, err = e.Navigate(ctx, []string{v}, false)
			if err != nil {
				return nil, err
			}
		}

		return e.NavigateSdcpbPath(ctx, pathElems[1:], false)
	}

	return nil, fmt.Errorf("navigating tree, reached %v but child %v does not exist", s.Path(), pathElems)
}

func (s *sharedEntryAttributes) tryLoadingDefault(ctx context.Context, path []string) (Entry, error) {

	schema, err := s.treeContext.treeSchemaCacheClient.GetSchema(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("error trying to load defaults for %s: %v", strings.Join(path, "->"), err)
	}

	var tv *sdcpb.TypedValue

	switch schem := schema.GetSchema().GetSchema().(type) {
	case *sdcpb.SchemaElem_Field:
		defaultVal := schem.Field.GetDefault()
		if defaultVal == "" {
			return nil, fmt.Errorf("error no default defined for %s", strings.Join(path, " -> "))
		}
		tv, err = utils.Convert(defaultVal, schem.Field.GetType())
		if err != nil {
			return nil, err
		}
	case *sdcpb.SchemaElem_Leaflist:
		listDefaults := schem.Leaflist.GetDefaults()
		tvlist := make([]*sdcpb.TypedValue, 0, len(listDefaults))
		for _, dv := range schem.Leaflist.GetDefaults() {
			tvelem, err := utils.Convert(dv, schem.Leaflist.GetType())
			if err != nil {
				return nil, fmt.Errorf("error converting default to typed value for %s, type: %s ; value: %s; err: %v", strings.Join(path, " -> "), schem.Leaflist.GetType().GetTypeName(), dv, err)
			}
			tvlist = append(tvlist, tvelem)
		}
		tv = &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_LeaflistVal{
				LeaflistVal: &sdcpb.ScalarArray{
					Element: tvlist,
				},
			},
		}
	default:
		return nil, fmt.Errorf("error no defaults defined for schema path: %s", strings.Join(path, "->"))
	}

	// convert value to []byte for cache insertion
	val, err := proto.Marshal(tv)
	if err != nil {
		return nil, err
	}

	upd := cache.NewUpdate(path, val, DefaultValuesPrio, DefaultsIntentName, 0)

	result, err := s.AddCacheUpdateRecursive(ctx, upd, false)
	if err != nil {
		return nil, fmt.Errorf("failed adding default value for %s to tree; %v", strings.Join(path, "/"), err)
	}

	return result, nil
}

// Navigate move through the tree, returns the Entry that is present under the given path
func (s *sharedEntryAttributes) Navigate(ctx context.Context, path []string, isRootPath bool) (Entry, error) {
	if len(path) == 0 {
		return s, nil
	}

	if isRootPath {
		return s.GetRoot().Navigate(ctx, path, false)
	}
	var err error
	switch path[0] {
	case ".":
		return s.Navigate(ctx, path[1:], false)
	case "..":
		return s.parent.Navigate(ctx, path[1:], false)
	default:
		e, exists := s.filterActiveChoiceCaseChilds()[path[0]]
		if !exists {
			e, err = s.tryLoadingDefault(ctx, append(s.Path(), path...))
			if err != nil {
				return nil, fmt.Errorf("navigating tree, reached %v but child %v does not exist, trying to load defaults yielded %v", s.Path(), path, err)
			}
			return e, nil
		}
		return e.Navigate(ctx, path[1:], false)
	}
}

func (s *sharedEntryAttributes) tryLoading(ctx context.Context, path []string) (Entry, error) {
	upd, err := s.treeContext.ReadRunning(ctx, append(s.Path(), path...))
	if err != nil {
		return nil, err
	}
	if upd == nil {
		return nil, fmt.Errorf("reached %v but child %s does not exist", s.Path(), path[0])
	}
	_, err = s.treeContext.root.AddCacheUpdateRecursive(ctx, upd, false)
	if err != nil {
		return nil, err
	}

	return s.childs[path[0]], nil
}

// GetHighestPrecedence goes through the whole branch and returns the new and updated cache.Updates.
// These are the updated that will be send to the device.
func (s *sharedEntryAttributes) GetHighestPrecedence(result LeafVariantSlice, onlyNewOrUpdated bool) LeafVariantSlice {
	// get the highes precedence LeafeVariant and add it to the list
	lv := s.leafVariants.GetHighestPrecedence(onlyNewOrUpdated)
	if lv != nil {
		result = append(result, lv)
	}

	// continue with childs. Childs are part of choices, process only the "active" (highes precedence) childs
	for _, c := range s.filterActiveChoiceCaseChilds() {
		result = c.GetHighestPrecedence(result, onlyNewOrUpdated)
	}
	return result
}

func (s *sharedEntryAttributes) GetRootBasedEntryChain() []Entry {
	if s.IsRoot() {
		return []Entry{}
	}
	return append(s.parent.GetRootBasedEntryChain(), s)
}

// getHighestPrecedenceValueOfBranch goes through all the child branches to find the highes
// precedence value (lowest priority value) for the entire branch and returns it.
func (s *sharedEntryAttributes) getHighestPrecedenceValueOfBranch() int32 {
	result := int32(math.MaxInt32)
	for _, e := range s.childs {
		if val := e.getHighestPrecedenceValueOfBranch(); val < result {
			result = val
		}
	}
	if val := s.leafVariants.GetHighestPrecedenceValue(); val < result {
		result = val
	}

	return result
}

func (s *sharedEntryAttributes) validateMandatoryWithKeys(level int, attribute string) error {
	if level == 0 {
		// first check if the mandatory value is set via the intent, e.g. part of the tree already
		v, existsInTree := s.filterActiveChoiceCaseChilds()[attribute]

		// if not the path exists in the tree and is not to be deleted, then lookup in the paths index of the store
		// and see if such path exists, if not raise the error
		if !(existsInTree && v.remainsToExist()) {
			if !s.treeContext.PathExists(append(s.Path(), attribute)) {
				return fmt.Errorf("%s: mandatory child %s does not exist", s.Path(), attribute)
			}
		}
		return nil
	}

	for _, c := range s.filterActiveChoiceCaseChilds() {
		err := c.validateMandatoryWithKeys(level-1, attribute)
		if err != nil {
			return err
		}
	}
	return nil
}

// Validate is the highlevel function to perform validation.
// it will multiplex all the different Validations that need to happen
func (s *sharedEntryAttributes) Validate(ctx context.Context, errchan chan<- error) {

	// recurse the call to the child elements
	wg := sync.WaitGroup{}
	defer wg.Wait()
	for _, c := range s.filterActiveChoiceCaseChilds() {
		wg.Add(1)
		go func(x Entry) {
			x.Validate(ctx, errchan)
			wg.Done()
		}(c)
	}

	// TODO: Validate Mandatory is skipped for now, until we have running
	// configuration information in the tree, to perform proper validation

	// // validate the mandatory statement on this entry
	// s.validateMandatory(errchan)
	if s.remainsToExist() {
		s.validateLeafRefs(ctx, errchan)
		s.validateLeafListMinMaxAttributes(errchan)
		s.validatePattern(errchan)
		s.validateMustStatements(ctx, errchan)
	}
}

// validateLeafListMinMaxAttributes validates the Min-, and Max-Elements attribute of the Entry if it is a Leaflists.
func (s *sharedEntryAttributes) validateLeafListMinMaxAttributes(errchan chan<- error) {
	if schema := s.schema.GetLeaflist(); schema != nil {
		if schema.MinElements > 0 {
			if lv := s.leafVariants.GetByOwner(s.treeContext.actualOwner); lv != nil {
				tv, err := lv.Update.Value()
				if err != nil {
					errchan <- fmt.Errorf("validating LeafList Min Attribute: %v", err)
				}
				if val := tv.GetLeaflistVal(); val != nil {
					// check minelements if set
					if schema.MinElements > 0 && len(val.GetElement()) < int(schema.GetMinElements()) {
						errchan <- fmt.Errorf("leaflist %s defines %d min-elements but only %d elements are present", s.Path().String(), schema.MinElements, len(val.GetElement()))
					}
					// check maxelements if set
					if len(val.GetElement()) > int(schema.GetMaxElements()) {
						errchan <- fmt.Errorf("leaflist %s defines %d max-elements but %d elements are present", s.Path().String(), schema.GetMaxElements(), len(val.GetElement()))
					}
				}
			}
		}
	}
}

// func (s *sharedEntryAttributes) validateUnique(errchan chan<- error) {
// 	if schema := s.schema.GetLeaflist(); schema != nil {
// 		if schema. {
// 			return
// 		}
// 	}
// }

// func (s *sharedEntryAttributes) validateLength(errchan chan<- error) {
// 	if schema := s.schema.GetField(); schema != nil {
// 		if schema.GetType().GetLength() == "" {
// 			return
// 		}

// 		lv := s.leafVariants.GetByOwner(s.treeContext.actualOwner)
// 		tv, err := lv.Value()
// 		if err != nil {
// 			errchan <- fmt.Errorf("failed reading value from %s LeafVariant %v: %w", s.Path(), lv, err)
// 			return
// 		}
// 		value := tv.GetStringVal()

// 	}
// }

func (s *sharedEntryAttributes) validatePattern(errchan chan<- error) {
	if schema := s.schema.GetField(); schema != nil {
		if len(schema.Type.Patterns) == 0 {
			return
		}
		lv := s.leafVariants.GetHighestPrecedence(false)
		tv, err := lv.Update.Value()
		if err != nil {
			errchan <- fmt.Errorf("failed reading value from %s LeafVariant %v: %w", s.Path(), lv, err)
			return
		}
		value := tv.GetStringVal()
		for _, pattern := range schema.Type.Patterns {
			if p := pattern.GetPattern(); p != "" {
				matched, err := regexp.MatchString(p, value)
				if err != nil {
					errchan <- fmt.Errorf("failed compiling regex %s defined for %s", p, s.Path())
					continue
				}
				if (!matched && !pattern.Inverted) || (pattern.GetInverted() && matched) {
					errchan <- fmt.Errorf("value %s of %s does not match regex %s (inverted: %t)", value, s.Path(), p, pattern.GetInverted())
				}
			}

		}
	}
}

func (s *sharedEntryAttributes) validateMustStatements(ctx context.Context, errchan chan<- error) {

	// if no schema, then there is nothing to be done, return
	if s.schema == nil {
		return
	}

	var mustStatements []*sdcpb.MustStatement
	switch schem := s.GetSchema().GetSchema().(type) {
	case *sdcpb.SchemaElem_Container:
		mustStatements = schem.Container.GetMustStatements()
	case *sdcpb.SchemaElem_Leaflist:
		mustStatements = schem.Leaflist.GetMustStatements()
	case *sdcpb.SchemaElem_Field:
		mustStatements = schem.Field.GetMustStatements()
	}

	for _, must := range mustStatements {
		// extract actual must statement
		exprStr := must.Statement
		// init a ProgramBuilder
		prgbuilder := xpath.NewProgBuilder(exprStr)
		// init an ExpressionLexer
		lexer := expr.NewExprLex(exprStr, prgbuilder, nil)
		// parse the provided Must-Expression
		lexer.Parse()
		prog, err := lexer.CreateProgram(exprStr)
		if err != nil {
			errchan <- err
			return
		}
		machine := xpath.NewMachine(exprStr, prog, exprStr)

		// run the must statement evaluation virtual machine
		yctx := xpath.NewCtxFromCurrent(ctx, machine, newYangParserEntryAdapter(ctx, s))
		yctx.SetDebug(false)

		res1 := yctx.Run()
		// retrieve the boolean result of the execution
		result, err := res1.GetBoolResult()
		if !result || err != nil {
			if err == nil {
				err = fmt.Errorf(must.Error)
			}
			if strings.Contains(err.Error(), "Stack underflow") {
				slog.Debug("stack underflow error: path=%v, mustExpr=%s", s.Path().String(), exprStr)
				continue
			}
			errchan <- err
			return
		}
	}
}

// validateMandatory validates that all the mandatory attributes,
// defined by the schema are present either in the tree or in the index.
func (s *sharedEntryAttributes) validateMandatory(errchan chan<- error) {
	if s.schema != nil {
		switch s.schema.GetSchema().(type) {
		case *sdcpb.SchemaElem_Container:
			for _, c := range s.schema.GetContainer().MandatoryChildren {
				err := s.validateMandatoryWithKeys(len(s.GetSchema().GetContainer().GetKeys()), c)
				if err != nil {
					errchan <- err
				}
			}
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
				v := child.getHighestPrecedenceValueOfBranch()
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

// markOwnerDelete Sets the delete flag on all the LeafEntries belonging to the given owner.
func (s *sharedEntryAttributes) markOwnerDelete(o string) {
	lvEntry := s.leafVariants.GetByOwner(o)
	// if an entry for the given user exists, mark it for deletion
	if lvEntry != nil {
		lvEntry.MarkDelete()
	}
	// recurse into childs
	for _, child := range s.childs {
		child.markOwnerDelete(o)
	}
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

// getKeyName checks if s is a key level element in the tree, if not an error is throw
// if it is a key level element, the name of the key is determined via the ancestor schemas
func (s *sharedEntryAttributes) getKeyName() (string, error) {
	// if the entry has a schema, it cannot be a Key attribute
	if s.schema != nil {
		return "", fmt.Errorf("error %s is a schema element, can only get KeyNames for key element", strings.Join(s.Path(), " "))
	}

	// get ancestro schema
	ancestorWithSchema, levelUp := s.GetFirstAncestorWithSchema()

	// only Contaieners have keys, so check for that
	switch sch := ancestorWithSchema.GetSchema().GetSchema().(type) {
	case *sdcpb.SchemaElem_Container:
		// return the name of the levelUp-1 key
		return sch.Container.GetKeys()[levelUp-1].Name, nil
	}

	// we probably called the function on a LeafList or LeafEntry which is not a valid call to be made.
	return "", fmt.Errorf("error LeafList and Field should not have keys %s", strings.Join(s.Path(), " "))
}

// AddCacheUpdateRecursive recursively adds the given cache.Update to the tree. Thereby creating all the entries along the path.
// if the entries along th path already exist, the existing entries are called to add the Update.
func (s *sharedEntryAttributes) AddCacheUpdateRecursive(ctx context.Context, c *cache.Update, new bool) (Entry, error) {
	idx := 0
	// if it is the root node, index remains == 0
	if s.parent != nil {
		idx = s.GetLevel()
	}
	// end of path reached, add LeafEntry
	// continue with recursive add otherwise
	if idx == len(c.GetPath()) {
		// Check if LeafEntry with given owner already exists
		if leafVariant := s.leafVariants.GetByOwner(c.Owner()); leafVariant != nil {
			if leafVariant.EqualSkipPath(c) {
				// it seems like the element was not deleted, so drop the delete flag
				leafVariant.Delete = false
			} else {
				// if a leafentry of the same owner exists with different value, mark it for update
				leafVariant.MarkUpdate(c)
			}
		} else {
			// if LeafVaraint with same owner does not exist, add the new entry
			s.leafVariants = append(s.leafVariants, NewLeafEntry(c, new, s))
		}

		return s, nil
	}

	var e Entry
	var err error
	var exists bool
	// if child does not exist, create Entry
	if e, exists = s.childs[c.GetPath()[idx]]; !exists {
		e, err = newEntry(ctx, s, c.GetPath()[idx], s.treeContext)
		if err != nil {
			return nil, err
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

	root := &RootEntry{
		sharedEntryAttributes: sea,
	}

	err = tc.SetRoot(sea)
	if err != nil {
		return nil, err
	}

	return root, nil
}

func (r *RootEntry) LoadIntendedStoreOwnerData(ctx context.Context, owner string, pathKeySet *PathSet) {
	tc := r.getTreeContext()
	ownerPaths := tc.GetPathsOfOwner(owner)

	// add the given paths as well
	ownerPaths.Join(pathKeySet)

	// Get all entries of the already existing intent
	highesCurrentCacheEntries := tc.ReadCurrentUpdatesHighestPriorities(ctx, ownerPaths.GetPaths(), 2)

	// add all the existing entries
	for _, entry := range highesCurrentCacheEntries {
		r.AddCacheUpdateRecursive(ctx, entry, false)
	}

	// Mark all the entries that belong to the owner / intent as deleted.
	// This is to allow for intent updates. We mark all existing entries for deletion up front.
	r.markOwnerDelete(owner)
}

// String returns the string representation of the Tree.
func (r *RootEntry) String() string {
	s := []string{}
	s = r.sharedEntryAttributes.StringIndent(s)
	return strings.Join(s, "\n")
}

// GetUpdatesForOwner returns the updates that have been calculated for the given intent / owner
func (r *RootEntry) GetUpdatesForOwner(owner string) UpdateSlice {
	// retrieve all the entries from the tree that belong to the given
	// Owner / Intent, skipping the once marked for deletion
	// this is to insert / update entries in the cache.
	return LeafEntriesToCacheUpdates(r.getByOwnerFiltered(owner, FilterNonDeletedButNewOrUpdated))
}

// GetDeletesForOwner returns the deletes that have been calculated for the given intent / owner
func (r *RootEntry) GetDeletesForOwner(owner string) PathSlices {
	// retrieve all entries from the tree that belong to the given user
	// and that are marked for deletion.
	// This is to cover all the cases where an intent was changed and certain
	// part of the config got deleted.
	deletesOwnerUpdates := LeafEntriesToCacheUpdates(r.getByOwnerFiltered(owner, FilterDeleted))
	// they are retrieved as cache.update, we just need the path for deletion from cache
	deletesOwner := make(PathSlices, 0, len(deletesOwnerUpdates))
	// so collect the paths
	for _, d := range deletesOwnerUpdates {
		deletesOwner = append(deletesOwner, d.GetPath())
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
func (r *RootEntry) GetDeletes(aggregatePaths bool) PathSlices {
	deletes := PathSlices{}
	return r.sharedEntryAttributes.GetDeletes(deletes, aggregatePaths)
}

// getTreeContext returns the handle to the TreeContext
func (r *RootEntry) getTreeContext() *TreeContext {
	return r.treeContext
}

func (r *RootEntry) GetAncestorSchema() (*sdcpb.SchemaElem, int) {
	return nil, 0
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
