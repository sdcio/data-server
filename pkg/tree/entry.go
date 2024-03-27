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
	// returns the Path as []string
	Path() []string
	// returns the last Path element, the name of the Entry
	PathName() string
	// Add a child entry
	AddChild(Entry) error
	// Add the given cache.Update to the tree
	AddCacheUpdateRecursive(*cache.Update) error
	// debug tree struct as indented string slice
	StringIndent(result []string) []string
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
}

func NewLeafEntry(c *cache.Update) *LeafEntry {
	return &LeafEntry{
		Update: c,
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
	LeafVariants []*LeafEntry
}

func (s *sharedEntryAttributes) GetLevel() int {
	return len(s.Path())
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

func newSharedEntryAttributes(parent Entry, pathElemName string) *sharedEntryAttributes {
	return &sharedEntryAttributes{
		Parent:       parent,
		PathElemName: pathElemName,
		Childs:       map[string]Entry{},
		LeafVariants: []*LeafEntry{},
	}
}

// RootEntry the root of the cache.Update tree
type RootEntry struct {
	*sharedEntryAttributes
}

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
		tv, err := l.Value()
		var v string
		if err != nil {
			v = err.Error()
		} else {
			v = tv.String()
		}
		result = append(result, fmt.Sprintf("%s -> Owner: %s, Priority: %d, Value: %s", strings.Repeat("  ", s.GetLevel()), l.Owner(), l.Priority(), v))
	}
	return result
}

func (r *sharedEntryAttributes) AddCacheUpdateRecursive(c *cache.Update) error {
	idx := 0
	// if it is the root node, index remains == 0
	if r.Parent != nil {
		idx = r.GetLevel()
	}
	// end of path reached, add LeafEntry
	// continue with recursive add otherwise
	if idx == len(c.GetPath()) {
		r.LeafVariants = append(r.LeafVariants, NewLeafEntry(c))
		return nil
	}

	var e Entry
	var exists bool
	// if child does not exist, create Entry
	if e, exists = r.Childs[c.GetPath()[idx]]; !exists {
		e = newEntry(r, c.GetPath()[idx])
	}
	e.AddCacheUpdateRecursive(c)

	return nil
}

func NewRootEntry() *RootEntry {
	return &RootEntry{
		sharedEntryAttributes: newSharedEntryAttributes(nil, ""),
	}
}

func (r *RootEntry) String() string {
	s := []string{}
	s = r.StringIndent(s)
	return strings.Join(s, "\n")
}
