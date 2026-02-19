package ops

import (
	"github.com/sdcio/data-server/pkg/tree/api"
	opsinterface "github.com/sdcio/data-server/pkg/tree/ops/interface"

	"github.com/sdcio/data-server/pkg/tree/types"
)

// GetByOwner returns all the LeafEntries that belong to a certain owner.
func GetByOwner(e opsinterface.Entry, owner string) api.LeafVariantSlice {
	result := api.LeafVariantSlice{}
	result = getByOwnerInternal(e, owner, result)
	return result
}

// getByOwnerInternal is the internal function that performs the actual retrieval of the LeafEntries by owner. It is called recursively to traverse the tree.
// It takes an additional result parameter that is used to accumulate the results during the recursive traversal. Since that LeafVariantSlice might grow during the traversal,
// it is returned as a new slice to ensure that the changes are reflected in the caller.
func getByOwnerInternal(e opsinterface.Entry, owner string, result api.LeafVariantSlice) api.LeafVariantSlice {
	lv := e.GetLeafVariants().GetByOwner(owner)
	if lv != nil {
		result = append(result, lv)
	}

	// continue with childs
	for _, c := range e.GetChilds(types.DescendMethodAll) {
		result = getByOwnerInternal(c, owner, result)
	}
	return result
}
