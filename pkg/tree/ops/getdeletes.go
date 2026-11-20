package ops

import (
	"github.com/sdcio/data-server/pkg/tree/api"

	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// GetDeletes calculates the paths to delete from the southbound device.
func GetDeletes(e api.Entry) (types.DeleteEntriesList, error) {
	return getDeletesInternal(e, make(types.DeleteEntriesList, 0))
}

func getDeletesInternal(e api.Entry, deletes types.DeleteEntriesList) (types.DeleteEntriesList, error) {
	if e.GetSchema() == nil {
		return getAggregatedDeletes(e, deletes)
	}
	return getRegularDeletes(e, deletes)
}

// getAggregatedDeletes emits a list-entry delete at the terminal key level when
// the entry is marked for deletion, and otherwise descends into child entries.
func getAggregatedDeletes(e api.Entry, deletes []types.DeleteEntry) ([]types.DeleteEntry, error) {
	var err error
	ancestor, level := GetFirstAncestorWithSchema(e)
	keys := GetSchemaKeys(ancestor)

	if len(keys) > 0 && level == len(keys) && e.ShouldDelete() {
		return append(deletes, e), nil
	}
	for _, c := range e.GetChildMap().GetAll() {
		deletes, err = getDeletesInternal(c, deletes)
		if err != nil {
			return nil, err
		}
	}
	return deletes, nil
}

func getRegularDeletes(e api.Entry, deletes types.DeleteEntriesList) (types.DeleteEntriesList, error) {
	var err error

	if e.ShouldDelete() && !e.IsRoot() && len(GetSchemaKeys(e)) == 0 {
		return append(deletes, e), nil
	}

	for _, elem := range e.ChoicesResolvers().GetDeletes() {
		deletes = append(deletes, types.NewDeleteEntryImpl(e.SdcpbPath().CopyPathAddElem(sdcpb.NewPathElem(elem, nil))))
	}

	for _, c := range e.GetChildMap().GetAll() {
		deletes, err = getDeletesInternal(c, deletes)
		if err != nil {
			return nil, err
		}
	}
	return deletes, nil
}
