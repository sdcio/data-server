package api

import (
	"sort"
)

type EntryMap map[string]Entry

func (e EntryMap) SortedKeys() []string {
	if len(e) == 0 {
		return nil
	}
	keys := make([]string, 0, len(e))
	for k := range e {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
