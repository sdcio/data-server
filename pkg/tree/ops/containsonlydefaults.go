package ops

import (
	"context"
	"slices"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/types"
)

// containsOnlyDefaults checks for presence containers, if only default values are present,
// such that the Entry should also be treated as a presence container
func ContainsOnlyDefaults(ctx context.Context, e api.Entry) bool {
	// if no schema is present, we must be in a key level
	if e.GetSchema() == nil {
		return false
	}
	contSchema := e.GetSchema().GetContainer()
	if contSchema == nil {
		return false
	}

	// only if length of childs is (more) compared to the number of
	// attributes carrying defaults, the presence condition can be met
	if len(e.GetChilds(types.DescendMethodAll)) > len(contSchema.ChildsWithDefaults) {
		return false
	}
	for k, v := range e.GetChilds(types.DescendMethodAll) {
		// check if child name is part of ChildsWithDefaults
		if !slices.Contains(contSchema.ChildsWithDefaults, k) {
			return false
		}
		// check if the value is the default value
		le, err := v.GetHighestPrecedenceLeafValue(ctx)
		if err != nil {
			return false
		}
		// if the owner is not Default return false
		if le.Owner() != consts.DefaultsIntentName {
			return false
		}
	}

	return true
}
