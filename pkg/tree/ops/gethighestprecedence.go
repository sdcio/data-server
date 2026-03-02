package ops

import (
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

// GetHighestPrecedence goes through the whole branch and returns the new and updated cache.Updates.
// These are the updated that will be send to the device.
func GetHighestPrecedence(s api.Entry, onlyNewOrUpdated bool, includeDefaults bool, includeExplicitDelete bool) api.LeafVariantSlice {
	result := make(api.LeafVariantSlice, 0)
	return getHighestPrecedenceInternal(s, result, onlyNewOrUpdated, includeDefaults, includeExplicitDelete)
}

func getHighestPrecedenceInternal(s api.Entry, result api.LeafVariantSlice, onlyNewOrUpdated bool, includeDefaults bool, includeExplicitDelete bool) api.LeafVariantSlice {
	// get the highes precedence LeafeVariant and add it to the list
	lv := s.GetLeafVariants().GetHighestPrecedence(onlyNewOrUpdated, includeDefaults, includeExplicitDelete)
	if lv != nil {
		result = append(result, lv)
	}

	// continue with childs. Childs are part of choices, process only the "active" (highes precedence) childs
	for _, c := range s.GetChilds(types.DescendMethodActiveChilds) {
		result = getHighestPrecedenceInternal(c, result, onlyNewOrUpdated, includeDefaults, includeExplicitDelete)
	}
	return result
}
