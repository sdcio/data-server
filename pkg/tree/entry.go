package tree

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/sdcio/data-server/pkg/cache"
	SchemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
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
	AddChild(Entry) error
	// AddCacheUpdateRecursive Add the given cache.Update to the tree
	AddCacheUpdateRecursive(u *cache.Update, new bool) error
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
}

func newEntry(parent Entry, pathElemName string) *EntryImpl {
	newEntry := &EntryImpl{
		sharedEntryAttributes: newSharedEntryAttributes(parent, pathElemName),
	}
	parent.AddChild(newEntry)
	return newEntry
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
	// isSchemaElement indicates if this reflects a Schema Element or a Key Element
	isSchemaElement bool
	// schema the schema element for this entry
	schema *sdcpb.SchemaElem
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

func (s *sharedEntryAttributes) AddChild(e Entry) error {
	// make sure Entry should not only hold LeafEntries
	if len(s.leafVariants) > 0 {
		return fmt.Errorf("cannot add child to %s since it holds Leafs", s)
	}
	// check the path of child is a subpath of s
	if !slices.Equal(s.Path(), e.Path()[:len(e.Path())-1]) {
		return fmt.Errorf("adding Child with diverging path, parent: %s, child: %s", s, strings.Join(e.Path()[:len(e.Path())-1], "/"))
	}
	s.childs[e.PathName()] = e
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

func newSharedEntryAttributes(parent Entry, pathElemName string) *sharedEntryAttributes {
	return &sharedEntryAttributes{
		parent:       parent,
		pathElemName: pathElemName,
		childs:       map[string]Entry{},
		leafVariants: newLeafVariants(),
	}
}

type LeafVariants []*LeafEntry

func newLeafVariants() LeafVariants {
	return make([]*LeafEntry, 0)
}

// ShouldDelete indicates if the entry should be deleted,
// since it is an entry that represents LeafsVariants but non
// of these are still valid.
func (lv LeafVariants) ShouldDelete() bool {
	// only procede if we have leave variants
	if len(lv) == 0 {
		return false
	}

	// go through all variants
	for _, e := range lv {
		// if there is a variant that is not marked as delete, no delete should be issued
		if !e.Delete {
			return false
		}
	}
	// return true otherwise
	return true
}

// GetHighesNewUpdated returns the LeafEntry with the highes priority
// nil if no leaf entry exists.
func (lv LeafVariants) GetHighesPrio(onlyIfPrioChanged bool) *LeafEntry {
	if len(lv) == 0 {
		return nil
	}

	var highest *LeafEntry
	var secondHighest *LeafEntry
	for _, e := range lv {
		// first entry set result to it
		// if it is not marked for deletion
		if highest == nil {
			highest = e
			continue
		}
		// on a result != nil that is then not marked for deletion
		// start comparing priorities and choose the one with the
		// higher prio (lower number)
		if highest.Priority() > e.Priority() {
			secondHighest = highest
			highest = e
		} else {
			// check if the update is at least higher prio (lower number) then the secondHighest
			if secondHighest == nil || secondHighest.Priority() > e.Priority() {
				secondHighest = e
			}
		}
	}

	// if it does not matter if the highes update is also
	// New or Updated return it
	if !onlyIfPrioChanged {
		if !highest.Delete {
			return highest
		}
		return secondHighest
	}

	// if the highes is not marked for deletion and new or updated (=PrioChanged) return it
	if !highest.Delete {
		if highest.IsNew || highest.IsUpdated {
			return highest
		}
		return nil
	}
	// otherwise if the secondhighest is not marked for deletion return it
	if secondHighest != nil && !secondHighest.Delete {
		return secondHighest
	}

	// otherwise return nil
	return nil

}

// GetByOwner returns the entry that is owned by the given owner,
// returns nil if no entry exists.
func (lv LeafVariants) GetByOwner(owner string) *LeafEntry {
	for _, e := range lv {
		if e.Owner() == owner {
			return e
		}
	}
	return nil
}

// RootEntry the root of the cache.Update tree
type RootEntry struct {
	*sharedEntryAttributes
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
	if lvEntry != nil {
		lvEntry.MarkDelete()
	}
	for _, child := range s.childs {
		child.MarkOwnerDelete(o)
	}
}

func (r *sharedEntryAttributes) AddCacheUpdateRecursive(c *cache.Update, new bool) error {
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
	var exists bool
	// if child does not exist, create Entry
	if e, exists = r.childs[c.GetPath()[idx]]; !exists {
		e = newEntry(r, c.GetPath()[idx])
	}
	e.AddCacheUpdateRecursive(c, new)

	return nil
}

func NewTreeRoot() *RootEntry {
	return &RootEntry{
		sharedEntryAttributes: newSharedEntryAttributes(nil, ""),
	}
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

func (r *RootEntry) getByOwner(owner string) []*LeafEntry {
	return r.getByOwnerFiltered(owner, Unfiltered)
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

// TreeWalkerSchemaRetriever returns an EntryVisitor, that populates the tree entries with the corresponding schema entries.
func TreeWalkerSchemaRetriever(ctx context.Context, scb SchemaClient.SchemaClientBound) EntryVisitor {
	// the schemaIndex is used as a lookup cache for Schema elements,
	// to prevent repetetive requests for the same schema element
	schemaIndex := map[string]*sdcpb.SchemaElem{}

	return func(s *sharedEntryAttributes) error {
		// if schema is already set, return early
		if s.schema != nil {
			return nil
		}

		// convert the []string path into sdcpb.path for schema retrieval
		sdcpbPath, err := scb.ToPath(ctx, s.Path())
		if err != nil {
			return err
		}

		// check if the actual path points to a key value (the last path element contains a key)
		// if so, we can skip querying the schema server
		if len(sdcpbPath.Elem) > 0 && len(sdcpbPath.Elem[len(sdcpbPath.Elem)-1].Key) > 0 {
			// s.schema remains nil
			// s.isSchemaElement remains false
			return nil
		}

		// convert the path into a keyless path, for schema index lookups.
		keylessPath := utils.SdcpbPathToKeylessString(sdcpbPath)

		// lookup schema in schemaindex, preventing consecutive gets from the schema server
		if v, exists := schemaIndex[keylessPath]; exists {
			// set the schema retrieved from SchemaIndex
			s.schema = v
			// we're done, schema is set, return
			return nil
		}

		// if schema wasn't found in index, go and fetch it
		schemaRsp, err := scb.GetSchema(ctx, sdcpbPath)
		if err != nil {
			return err
		}

		// store schema in schemaindex for next lookup
		schemaIndex[keylessPath] = schemaRsp.GetSchema()
		// set the sharedEntryAttributes related values
		s.schema = schemaRsp.GetSchema()
		s.isSchemaElement = true
		return nil
	}
}
