package ops

import (
	"context"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

func GetDeviations(ctx context.Context, e api.Entry, ch chan<- *types.DeviationEntry, activeCase bool) {
	evalLeafvariants := true
	// if s is a presence container but has active childs, it should not be treated as a presence
	// container, hence the leafvariants should not be processed. For presence container with
	// childs the TypedValue.empty_val in the presence container is irrelevant.
	if e.GetSchema().GetContainer().GetIsPresence() && len(e.GetChilds(types.DescendMethodActiveChilds)) > 0 {
		evalLeafvariants = false
	}

	if evalLeafvariants {
		// calculate Deviation on the LeafVariants
		e.GetLeafVariants().GetDeviations(ctx, ch, activeCase)
	}

	// get all active childs
	activeChilds := e.GetChilds(types.DescendMethodActiveChilds)

	// iterate through all childs
	for cName, c := range e.GetChildMap().GetAll() {
		// check if c is a active child (choice / case)
		_, isActiveChild := activeChilds[cName]
		// recurse the call
		GetDeviations(ctx, c, ch, isActiveChild)
	}
}
