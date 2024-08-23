package tree

import "github.com/sdcio/data-server/pkg/cache"

type LeafVariantSlice []*LeafEntry

func (lvs LeafVariantSlice) ToCacheUpdateSlice() []*cache.Update {
	result := make([]*cache.Update, 0, len(lvs))
	for _, x := range lvs {
		result = append(result, x.Update)
	}
	return result
}
