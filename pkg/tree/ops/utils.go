package ops

import (
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

func getListEntrySortFunc(parent api.Entry) func(a, b api.Entry) int {
	// return the comparison function
	return func(a, b api.Entry) int {
		keys := GetSchemaKeys(parent)
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
			aLvSlice := GetHighestPrecedence(achild, false, true, true)
			bLvSlice := GetHighestPrecedence(bchild, false, true, true)

			// A key leaf can legitimately end up without any LeafVariant (e.g. a
			// structural/placeholder entry created while navigating the tree without
			// a value ever being set for it). We cannot compare such entries, so treat
			// them as equal on this key, just like the "doesn't exist" case above.
			if len(aLvSlice) == 0 || len(bLvSlice) == 0 {
				return 0
			}

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
