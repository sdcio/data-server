package tree

import (
	"math"
	"strings"
)

type TreeContext struct {
	storeIndex            map[string]UpdateSlice // contains the
	treeSchemaCacheClient TreeSchemaCacheClient
	actualOwner           string
}

func NewTreeContext(tscc TreeSchemaCacheClient, actualOwner string) *TreeContext {
	return &TreeContext{
		treeSchemaCacheClient: tscc,
		actualOwner:           actualOwner,
	}
}

func (t *TreeContext) GetActualOwner() string {
	return t.actualOwner
}

func (t *TreeContext) PathExists(path []string) bool {
	_, exists := t.storeIndex[strings.Join(path, KeysIndexSep)]
	return exists
}

func (t *TreeContext) GetBranchesHighesPrecedence(path []string, filters ...CacheUpdateFilter) int32 {
	result := int32(math.MaxInt32)
	pathKey := strings.Join(path, KeysIndexSep)

	// TODO: Improve this, since it is probably an expensive operation
	for key, entries := range t.storeIndex {
		if strings.HasPrefix(key, pathKey) {
			if prio := entries.GetLowestPriorityValue(filters); prio < result {
				result = prio
			}
		}
	}
	return result
}

func (t *TreeContext) GetPathsOfOwner(owner string) *PathSet {
	p := NewPathSet()
	for _, keyMeta := range t.storeIndex {
		for _, k := range keyMeta {
			if k.Owner() == owner {
				// if the key is not yet listed in the keys slice, add it otherwise skip
				p.AddPath(k.GetPath())
			}
		}
	}
	return p
}

func (t *TreeContext) SetStoreIndex(si map[string]UpdateSlice) {
	t.storeIndex = si
}
