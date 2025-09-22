package tree

import (
	"iter"
	"slices"
	"sync"
)

type childMap struct {
	c  map[string]Entry
	mu sync.RWMutex
}

func newChildMap() *childMap {
	return &childMap{
		c: map[string]Entry{},
	}
}

func (c *childMap) Items() iter.Seq2[string, Entry] {
	return func(yield func(string, Entry) bool) {
		for i, v := range c.c {
			if !yield(i, v) {
				return
			}
		}
	}
}

func (c *childMap) DeleteChilds(names []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, name := range names {
		delete(c.c, name)
	}
}

func (c *childMap) DeleteChild(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.c, name)
}

func (c *childMap) Add(e Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.c[e.PathName()] = e
}

func (c *childMap) GetEntry(s string) (e Entry, exists bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, exists = c.c[s]
	return e, exists
}

func (c *childMap) GetAllSorted() []Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	childNames := make([]string, 0, len(c.c))
	for name := range c.c {
		childNames = append(childNames, name)
	}
	slices.Sort(childNames)

	result := make([]Entry, 0, len(c.c))
	// range over children
	for _, childName := range childNames {
		result = append(result, c.c[childName])
	}

	return result
}

func (c *childMap) GetAll() map[string]Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]Entry, len(c.c))
	for k, v := range c.c {
		result[k] = v
	}
	return result
}

func (c *childMap) GetKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]string, 0, c.Length())
	for k := range c.c {
		result = append(result, k)
	}
	return result
}

func (c *childMap) Length() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.c)
}
