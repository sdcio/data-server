package ops

import (
	"github.com/sdcio/data-server/pkg/tree/api"

	"github.com/sdcio/data-server/pkg/tree/types"
)

// GetByOwner returns all the LeafEntries that belong to a certain owner.
func GetByOwner(e api.Entry, owner string) api.LeafVariantSlice {
	result := api.LeafVariantSlice{}
	result = getByOwnerInternal(e, owner, result)
	return result
}

// getByOwnerInternal is the internal function that performs the actual retrieval of the LeafEntries by owner. It is called recursively to traverse the tree.
// It takes an additional result parameter that is used to accumulate the results during the recursive traversal. Since that LeafVariantSlice might grow during the traversal,
// it is returned as a new slice to ensure that the changes are reflected in the caller.
func getByOwnerInternal(e api.Entry, owner string, result api.LeafVariantSlice, f ...api.LeafEntryFilter) api.LeafVariantSlice {
	lv := e.GetLeafVariants().GetByOwner(owner)
	add := true
	if lv != nil {
		for _, filter := range f {
			// if the filter yields false, skip
			if !filter(lv) {
				add = false
				break
			}
		}
		if add {
			result = append(result, lv)
		}
	}

	// continue with childs
	for _, c := range e.GetChilds(types.DescendMethodAll) {
		result = getByOwnerInternal(c, owner, result, f...)
	}
	return result
}

// getByOwnerFiltered returns the Tree content filtered by owner, whilst allowing to filter further
// via providing additional LeafEntryFilter
func GetByOwnerFiltered(e api.Entry, owner string, f ...api.LeafEntryFilter) api.LeafVariantSlice {
	result := api.LeafVariantSlice{}
	result = getByOwnerInternal(e, owner, result, f...)
	return result
}
