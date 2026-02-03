package tree

import "github.com/sdcio/data-server/pkg/tree/types"

func getListEntrySortFunc(parent Entry) func(a, b Entry) int {
	// return the comparison function
	return func(a, b Entry) int {
		keys := parent.GetSchemaKeys()
		var cmpResult int
		for _, v := range keys {
			achild, exists := a.GetChilds(types.DescendMethodAll)[v]
			if !exists {
				return 0
			}
			bchild, exists := b.GetChilds(types.DescendMethodAll)[v]
			if !exists {
				return 0
			}
			aLvSlice := achild.GetHighestPrecedence(LeafVariantSlice{}, false, true, true)
			bLvSlice := bchild.GetHighestPrecedence(LeafVariantSlice{}, false, true, true)

			aEntry := aLvSlice[0]
			bEntry := bLvSlice[0]

			cmpResult = aEntry.Value().Cmp(bEntry.Value())
			if cmpResult != 0 {
				return cmpResult
			}
		}
		return 0
	}
}
