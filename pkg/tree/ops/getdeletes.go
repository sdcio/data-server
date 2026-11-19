package ops

import (
	"github.com/sdcio/data-server/pkg/tree/api"

	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// GetDeletes calculate the deletes that need to be send to the device.
func GetDeletes(e api.Entry, aggregatePaths bool) (types.DeleteEntriesList, error) {
	return getDeletesInternal(e, make(types.DeleteEntriesList, 0), aggregatePaths)
}

// getDeletesInternal is the internal function to calculate deletes, it is called recursively on the tree and performs the actual calculation.
func getDeletesInternal(e api.Entry, deletes types.DeleteEntriesList, aggregatePaths bool) (types.DeleteEntriesList, error) {

	// if the actual level has no schema assigned we're on a key level
	// element. Hence we try deletion via aggregation
	if e.GetSchema() == nil && aggregatePaths {
		return getAggregatedDeletes(e, deletes, aggregatePaths)
	}

	// else perform regular deletion
	return getRegularDeletes(e, deletes, aggregatePaths)
}

// getAggregatedDeletes emits a list-entry delete at the terminal key level when
// the entry is marked for deletion, and otherwise descends into child entries.
func getAggregatedDeletes(e api.Entry, deletes []types.DeleteEntry, aggregatePaths bool) ([]types.DeleteEntry, error) {
	var err error
	ancestor, level := GetFirstAncestorWithSchema(e)
	keys := GetSchemaKeys(ancestor)

	if len(keys) > 0 {
		if level == len(keys) && e.ShouldDelete() {
			return append(deletes, e), nil
		}
		for _, c := range e.GetChildMap().GetAll() {
			deletes, err = getDeletesInternal(c, deletes, aggregatePaths)
			if err != nil {
				return nil, err
			}
		}
		return deletes, nil
	}
	return getRegularDeletes(e, deletes, aggregatePaths)
}

// getRegularDeletes performs deletion calculation on elements that have a schema attached.
func getRegularDeletes(e api.Entry, deletes types.DeleteEntriesList, aggregate bool) (types.DeleteEntriesList, error) {
	var err error

	if e.ShouldDelete() && !e.IsRoot() && e.GetSchema() != nil && len(GetSchemaKeys(e)) == 0 {
		return append(deletes, e), nil
	}

	for _, elem := range e.ChoicesResolvers().GetDeletes() {
		deletes = append(deletes, types.NewDeleteEntryImpl(e.SdcpbPath().CopyPathAddElem(sdcpb.NewPathElem(elem, nil))))
	}

	for _, c := range e.GetChildMap().GetAll() {
		deletes, err = getDeletesInternal(c, deletes, aggregate)
		if err != nil {
			return nil, err
		}
	}
	return deletes, nil
}
