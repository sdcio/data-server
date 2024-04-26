package tree

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sdcio/data-server/pkg/cache"
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
	// GetHighesPrio return the new cache.Update entried from the tree that are the highes priority and new == true.
	// It will append to the given list and provide a new pointer to the slice
	GetHighesPrio([]*cache.Update) []*cache.Update
	// GetByOwner returns the branches Updates by owner
	GetByOwner(owner string, result []*LeafEntry) []*LeafEntry
	// MarkOwnerDelete Sets the delete flag on all the LeafEntries belonging to the given owner.
	MarkOwnerDelete(o string)
	// GetDeletes returns the cache-updates that are not updated, have no lower priority value left and hence should be deleted completely
	GetDeletes([][]string) [][]string
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
	// the parent Entry, nil for the root Entry
	Parent Entry
	// The path elements name the entry represents
	PathElemName string
	// Childs mutual exclusive with LeafVariants
	Childs map[string]Entry
	// LeafVariants mutual exclusive with Childs
	// If Entry is a leaf it can hold multiple LeafVariants
	LeafVariants LeafVariants
}

func (s *sharedEntryAttributes) GetLevel() int {
	return len(s.Path())
}

func (s *sharedEntryAttributes) GetDeletes(deletes [][]string) [][]string {
	if s.LeafVariants.ShouldDelete() {
		deletes = append(deletes, s.LeafVariants[0].GetPath())
	}
	for _, e := range s.Childs {
		deletes = e.GetDeletes(deletes)
	}
	return deletes
}

func (s *sharedEntryAttributes) GetByOwner(owner string, result []*LeafEntry) []*LeafEntry {
	lv := s.LeafVariants.GetByOwner(owner)
	if lv != nil {
		result = append(result, lv)
	}

	// continue with childs
	for _, c := range s.Childs {
		result = c.GetByOwner(owner, result)
	}
	return result
}

func (s *sharedEntryAttributes) Path() []string {
	// special handling for root node
	if s.Parent == nil {
		return []string{}
	}
	return append(s.Parent.Path(), s.PathElemName)
}

func (s *sharedEntryAttributes) PathName() string {
	return s.PathElemName
}

func (s *sharedEntryAttributes) String() string {
	return strings.Join(s.Parent.Path(), "/")
}

func (s *sharedEntryAttributes) AddChild(e Entry) error {
	// make sure Entry should not only hold LeafEntries
	if len(s.LeafVariants) > 0 {
		return fmt.Errorf("cannot add child to %s since it holds Leafs", s)
	}
	// check the path of child is a subpath of s
	if !slices.Equal(s.Path(), e.Path()[:len(e.Path())-1]) {
		return fmt.Errorf("adding Child with diverging path, parent: %s, child: %s", s, strings.Join(e.Path()[:len(e.Path())-1], "/"))
	}
	s.Childs[e.PathName()] = e
	return nil
}

func (s *sharedEntryAttributes) GetHighesPrio(result []*cache.Update) []*cache.Update {
	// try to get get the highes prio LeafVariant make sure it exists (!= nil)
	// and add it to the result if it is NEW
	lv := s.LeafVariants.GetHighes()
	if lv != nil {
		result = append(result, lv.Update)
	}

	// continue with childs
	for _, c := range s.Childs {
		result = c.GetHighesPrio(result)
	}
	return result
}

func newSharedEntryAttributes(parent Entry, pathElemName string) *sharedEntryAttributes {
	return &sharedEntryAttributes{
		Parent:       parent,
		PathElemName: pathElemName,
		Childs:       map[string]Entry{},
		LeafVariants: newLeafVariants(),
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

// GetHighes returns the LeafEntry with the highes priority
// nil if no leaf entry exists.
func (lv LeafVariants) GetHighes() *LeafEntry {
	var result *LeafEntry
	for _, e := range lv {
		// first entry set result to it
		// if it is not marked for deletion
		if result == nil && !e.Delete {
			result = e
			continue
		}
		// on second start comparing
		// compare priorities only if the entry is not marked for deletion
		if !e.Delete && result.Priority() > e.Priority() {
			result = e
		}
	}
	return result
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
	result = append(result, strings.Repeat("  ", s.GetLevel())+s.PathElemName)

	// ranging over children and LeafVariants
	// then should be mutual exclusive, either a node has children or LeafVariants

	// range over children
	for _, c := range s.Childs {
		result = c.StringIndent(result)
	}
	// range over LeafVariants
	for _, l := range s.LeafVariants {
		result = append(result, fmt.Sprintf("%s -> %s", strings.Repeat("  ", s.GetLevel()), l.String()))
	}
	return result
}

// MarkOwnerDelete Sets the delete flag on all the LeafEntries belonging to the given owner.
func (s *sharedEntryAttributes) MarkOwnerDelete(o string) {
	lvEntry := s.LeafVariants.GetByOwner(o)
	if lvEntry != nil {
		lvEntry.MarkDelete()
	}
	for _, child := range s.Childs {
		child.MarkOwnerDelete(o)
	}
}

func (r *sharedEntryAttributes) AddCacheUpdateRecursive(c *cache.Update, new bool) error {
	idx := 0
	// if it is the root node, index remains == 0
	if r.Parent != nil {
		idx = r.GetLevel()
	}
	// end of path reached, add LeafEntry
	// continue with recursive add otherwise
	if idx == len(c.GetPath()) {
		// Check if LeafEntry with given owner already exists
		if leafVariant := r.LeafVariants.GetByOwner(c.Owner()); leafVariant != nil {
			if leafVariant.EqualSkipPath(c) {
				// it seems like the element was not deleted, so drop the delete flag
				leafVariant.Delete = false
			} else {
				// if a leafentry of the same owner exists with different value, mark it for update
				leafVariant.MarkUpdate(c)
			}
		} else {
			// if LeafVaraint with same owner does not exist, add the new entry
			r.LeafVariants = append(r.LeafVariants, NewLeafEntry(c, new))
		}
		return nil
	}

	var e Entry
	var exists bool
	// if child does not exist, create Entry
	if e, exists = r.Childs[c.GetPath()[idx]]; !exists {
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

func (r *RootEntry) GetHighesPrio() []*cache.Update {
	return r.sharedEntryAttributes.GetHighesPrio(make([]*cache.Update, 0))
}

func (r *RootEntry) GetDeletes() [][]string {
	deletes := [][]string{}
	return r.sharedEntryAttributes.GetDeletes(deletes)
}

func (r *RootEntry) GetByOwner(owner string) []*LeafEntry {
	return r.GetByOwnerFiltered(owner, Unfiltered)
}

func (r *RootEntry) GetByOwnerFiltered(owner string, f ...LeafEntryFilter) []*LeafEntry {
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
