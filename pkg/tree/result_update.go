package tree

import "github.com/sdcio/data-server/pkg/cache"

type ResultUpdate struct {
	treeEntry Entry
	update    *cache.Update
}

func (r *ResultUpdate) GetTreeEntry() Entry {
	return r.treeEntry
}

func (r *ResultUpdate) GetUpdate() *cache.Update {
	return r.update
}
