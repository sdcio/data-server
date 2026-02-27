package ops

import (
	"fmt"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/sdc-protos/tree_persist"
)

var (
	ErrorIntentNotPresent = fmt.Errorf("intent not present")
)

func TreeExport(e api.Entry, owner string, priority int32) (*tree_persist.Intent, error) {
	treeExport, err := treeExportLevel(e, owner)
	if err != nil {
		return nil, err
	}

	explicitDeletes := e.GetTreeContext().ExplicitDeletes().GetByIntentName(owner).ToPathSlice()

	var rootExportEntry *tree_persist.TreeElement
	if len(treeExport) != 0 {
		rootExportEntry = treeExport[0]
	}

	if rootExportEntry != nil || len(explicitDeletes) > 0 {
		return &tree_persist.Intent{
			IntentName:      owner,
			Root:            rootExportEntry,
			Priority:        priority,
			NonRevertive:    e.GetTreeContext().NonRevertiveInfo().IsGenerallyNonRevertive(owner),
			ExplicitDeletes: explicitDeletes,
		}, nil
	}
	return nil, ErrorIntentNotPresent
}

// treeExportLevel exports the tree starting at the given entry for the given owner. It returns a slice of TreeElements which represent the exported tree.
func treeExportLevel(e api.Entry, owner string) ([]*tree_persist.TreeElement, error) {
	var lvResult []byte
	var childResults []*tree_persist.TreeElement
	var err error

	le := e.GetLeafVariants().GetByOwner(owner)

	if le != nil && !le.Delete {
		lvResult, err = le.ValueAsBytes()
		if err != nil {
			return nil, err
		}
	}

	if len(GetSchemaKeys(e)) > 0 {
		children, err := e.FilterChilds(nil)
		if err != nil {
			return nil, err
		}
		result := []*tree_persist.TreeElement{}
		for _, c := range children {
			childexport, err := treeExportLevel(c, owner)
			if err != nil {
				return nil, err
			}
			if len(childexport) == 0 {
				// no childs belonging to the given owner
				continue
			}
			if len(childexport) > 1 {
				return nil, fmt.Errorf("unexpected value")
			}
			childexport[0].Name = e.PathName()

			result = append(result, childexport...)
		}
		if len(result) > 0 {
			return result, nil
		}
	} else {
		for _, c := range e.GetChildMap().GetAll() {
			childExport, err := treeExportLevel(c, owner)
			if err != nil {
				return nil, err
			}
			if len(childExport) > 0 {
				childResults = append(childResults, childExport...)
			}

		}
		if lvResult != nil || len(childResults) > 0 {
			return []*tree_persist.TreeElement{
				{
					Name:        e.PathName(),
					Childs:      childResults,
					LeafVariant: lvResult,
				},
			}, nil
		}
	}

	return nil, nil
}
