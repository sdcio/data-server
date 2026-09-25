package api

import (
	"sort"
)

// EntryMap keys are NodeIdentity.MapKey() strings.
type EntryMap map[string]Entry

// Get looks up a child by node identity.
func (e EntryMap) Get(id NodeIdentity) (Entry, bool) {
	ent, ok := e[id.MapKey()]
	return ent, ok
}

func (e EntryMap) SortedKeys() []string {
	keys := make([]string, 0, len(e))
	for k := range e {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
