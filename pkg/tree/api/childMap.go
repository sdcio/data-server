package api

import (
	"maps"
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

func (c *ChildMap) DeleteChilds(ids []NodeIdentity) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range ids {
		delete(c.c, id.MapKey())
	}
}

func (c *ChildMap) DeleteChild(id NodeIdentity) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.c, id.MapKey())
}

// AddOrGet adds the entry if no entry with the same path name exists.
// If an entry already exists, it returns the existing (canonical) entry.
// This provides a safe get-or-create primitive for concurrent callers.
func (c *ChildMap) AddOrGet(e Entry) Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := e.Identity().MapKey()
	if existing, ok := c.c[k]; ok {
		return existing
	}
	c.c[k] = e
	return e
}

func (c *ChildMap) GetEntry(id NodeIdentity) (e Entry, exists bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, exists = c.c[id.MapKey()]
	return e, exists
}

func (c *ChildMap) GetAllSorted() []Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	childNames := c.SortedKeys()
	result := make([]Entry, 0, len(c.c))
	// range over children
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

// GetAll returns a copy of the map of all entries in the child map
func (c *ChildMap) GetAll() map[string]Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]Entry, len(c.c))
	maps.Copy(result, c.c)
	return result
}

// GetKeys returns the keys of the child map
func (c *ChildMap) GetKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]string, 0, c.Length())
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
