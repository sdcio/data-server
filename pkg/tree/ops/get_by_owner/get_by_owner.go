package getbyowner

import "github.com/sdcio/data-server/pkg/tree/api"

// GetByOwner returns all the LeafEntries that belong to a certain owner.
func GetByOwner(e Entry, owner string, result []*api.LeafEntry) api.LeafVariantSlice {
	lv := e.GetLeafVariants().GetByOwner(owner)
	if lv != nil {
		result = append(result, lv)
	}

	// continue with childs
	for _, c := range e.GetChilds() {
		result = GetByOwner(c, owner, result)
	}
	return result
}

type Entry interface {
	GetLeafVariants() *api.LeafVariants
	GetChilds() []Entry
}
