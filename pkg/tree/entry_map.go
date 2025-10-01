package tree

import "slices"

type EntryMap map[string]Entry

func (e EntryMap) SortedSlice() []Entry {
	s := make([]string, 0, len(e))

	for k := range e {
		s = append(s, k)
	}

	slices.Sort(s)

	result := make([]Entry, 0, len(e))
	for _, k := range s {
		result = append(result, e[k])
	}

	return result
}
