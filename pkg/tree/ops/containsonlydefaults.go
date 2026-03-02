package ops

import (
	"slices"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/types"
)

// ContainsOnlyDefaults checks if the given entry is a container that only contains leafvariants with owner defaults,
// and that all childs are in the list of childs with defaults defined in the container schema. Containers without
// childs return true, while non-container entries or entries without schema return false. It returns true if the
// entry is a container with only default values, and false otherwise.
func ContainsOnlyDefaults(e api.Entry) bool {
	schema := e.GetSchema()
	if schema == nil {
		return false
	}

	contSchema := schema.GetContainer()
	if contSchema == nil {
		return false
	}

	// if the amount of childs is higher than the amount of childs with defaults, it can't be only defaults
	childs := e.GetChilds(types.DescendMethodAll)
	if len(childs) > len(contSchema.ChildsWithDefaults) {
		return false
	}

	// check if all childs are in the list of childs with defaults, and that they only have a leafvariant with owner defaults
	for k, v := range childs {
		if !slices.Contains(contSchema.ChildsWithDefaults, k) {
			return false
		}
		le := v.GetLeafVariants().GetHighestPrecedence(false, true, false)
		if le == nil {
			return false
		}
		if le.Owner() != consts.DefaultsIntentName {
			return false
		}
	}

	return true
}
