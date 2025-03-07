package tree

import "github.com/sdcio/data-server/pkg/tree/types"

type LeafVariantSlice []*LeafEntry

func (lvs LeafVariantSlice) ToUpdateSlice() types.UpdateSlice {
	result := make([]*types.Update, 0, len(lvs))
	for _, x := range lvs {
		result = append(result, x.Update)
	}
	return result
}
