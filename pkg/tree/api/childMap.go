package api

import (
	"maps"
	"slices"
	"sort"
	"sync"
)

type ChildMap struct {
	c  map[string]Entry
	mu sync.RWMutex
}

func NewChildMap() *ChildMap {
	return &ChildMap{
		c: map[string]Entry{},
	}
}

// NewChildMapWithEntries creates a ChildMap with initial entries (for testing)
func NewChildMapWithEntries(entries map[string]Entry) *ChildMap {
	c := &ChildMap{
		c: make(map[string]Entry, len(entries)),
	}
	for k, v := range entries {
		c.c[k] = v
	}
	return c
}

func (c *ChildMap) DeleteChilds(names []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, name := range names {
		delete(c.c, name)
	}
}

func (c *ChildMap) DeleteChild(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.c, name)
}

// AddOrGet adds the entry if no entry with the same path name exists.
// If an entry already exists, it returns the existing (canonical) entry.
// This provides a safe get-or-create primitive for concurrent callers.
func (c *ChildMap) AddOrGet(e Entry) Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := e.PathName()
	if existing, ok := c.c[k]; ok {
		return existing
	}
	c.c[k] = e
	return e
}

func (c *ChildMap) GetEntry(s string) (e Entry, exists bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, exists = c.c[s]
	return e, exists
}

func (c *ChildMap) GetAllSorted() []Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.c) == 0 {
		return nil
	}

	childNames := make([]string, 0, len(c.c))
	for childName := range c.c {
		childNames = append(childNames, childName)
	}
	sort.Strings(childNames)

	result := make([]Entry, 0, len(childNames))
	for _, childName := range childNames {
		result = append(result, c.c[childName])
	}

	return result
}

// ForEach iterates over all children while holding a read lock.
func (c *ChildMap) ForEach(fn func(name string, e Entry)) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for name, child := range c.c {
		fn(name, child)
	}
}

// Snapshot copies children into a slice under the read lock.
// Names in skip are omitted. The lock is released before the caller ranges
// the slice, so a later delete (write lock) cannot deadlock against the walk.
// An empty child map returns nil without allocating.
func (c *ChildMap) Snapshot(skip []string) []Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.c) == 0 {
		return nil
	}

	result := make([]Entry, 0, len(c.c))
	for name, child := range c.c {
		if len(skip) > 0 && slices.Contains(skip, name) {
			continue
		}
		result = append(result, child)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// GetAll returns a copy of the map of all entries in the child map.
// An entry with no children returns nil without allocating.
func (c *ChildMap) GetAll() map[string]Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.c) == 0 {
		return nil
	}

	result := make(map[string]Entry, len(c.c))
	maps.Copy(result, c.c)
	return result
}

// GetKeys returns the keys of the child map
func (c *ChildMap) GetKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.c) == 0 {
		return nil
	}

	result := make([]string, 0, len(c.c))
	for k := range c.c {
		result = append(result, k)
	}
	return result
}

// Length returns the number of entries in the child map
func (c *ChildMap) Length() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.c)
}

// SortedKeys returns the keys of the child map in sorted order
func (c *ChildMap) SortedKeys() []string {
	keys := c.GetKeys()
	sort.Strings(keys)
	return keys
}
