package tree

import (
	"sort"
)

type EntryMap map[string]Entry

func (e EntryMap) SortedKeys() []string {
	keys := make([]string, 0, len(e))
	for k := range e {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
