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

// getAggregatedDeletes is called on levels that have no schema attached, meaning key schemas.
// here we might delete the whole branch of the tree, if all key elements are being deleted
// if not, we continue with regular deletes
func getAggregatedDeletes(e api.Entry, deletes []types.DeleteEntry, aggregatePaths bool) ([]types.DeleteEntry, error) {
	var err error
	// we take a look into the level(s) up
	// trying to get the schema
	ancestor, level := GetFirstAncestorWithSchema(e)

	// check if the first schema on the path upwards (parents)
	// has keys defined (meaning is a contianer with keys)
	keys := GetSchemaKeys(ancestor)

	// if keys exist and we're on the last level of the keys, validate
	// if aggregation can happen
	if len(keys) > 0 && level == len(keys) {
		doAggregateDelete := true
		// check the keys for deletion
		for _, n := range keys {
			c, exists := e.GetChildMap().GetEntry(n)
			// these keys should always exist, so for now we do not catch the non existing key case
			if exists && !c.ShouldDelete() {
				// if not all the keys are marked for deletion, we need to revert to regular deletion
				doAggregateDelete = false
				break
			}
		}
		// if aggregate delete is possible do it
		if doAggregateDelete {
			// by adding the key path to the deletes
			deletes = append(deletes, e)
		} else {
			// otherwise continue with deletion on the childs.
			for _, c := range e.GetChildMap().GetAll() {
				deletes, err = getDeletesInternal(c, deletes, aggregatePaths)
				if err != nil {
					return nil, err
				}
			}
		}
		return deletes, nil
	}
	return getRegularDeletes(e, deletes, aggregatePaths)
}

// getRegularDeletes performs deletion calculation on elements that have a schema attached.
func getRegularDeletes(e api.Entry, deletes types.DeleteEntriesList, aggregate bool) (types.DeleteEntriesList, error) {
	var err error

	if e.ShouldDelete() && !e.IsRoot() && len(GetSchemaKeys(e)) == 0 {
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
