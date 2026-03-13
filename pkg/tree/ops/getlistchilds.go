package ops

import (
	"fmt"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

// GetListChilds collects all the childs of the list. In the tree we store them seperated into their key branches.
// this is collecting all the last level key entries.
func GetListChilds(e api.Entry) ([]api.Entry, error) {
	if e.GetSchema() == nil {
		return nil, fmt.Errorf("error GetListChilds() non schema level %s", e.SdcpbPath().ToXPath(false))
	}
	if e.GetSchema().GetContainer() == nil {
		return nil, fmt.Errorf("error GetListChilds() not a Container %s", e.SdcpbPath().ToXPath(false))
	}
	keys := GetSchemaKeys(e)
	if len(keys) == 0 {
		return nil, fmt.Errorf("error GetListChilds() not a List Container %s", e.SdcpbPath().ToXPath(false))
	}

	current := []api.Entry{e}

	// Collect descendants level-by-level through key hierarchy
	for range keys {
		// Cache children and calculate total count
		childrenList := make([]api.EntryMap, len(current))
		totalChildren := 0
		// Iterate current level, collect children and count total
		for i, entry := range current {
			children := entry.GetChilds(types.DescendMethodAll)
			childrenList[i] = children
			totalChildren += len(children)
		}

		// Allocate result slice with total count and populate it
		next := make([]api.Entry, totalChildren)
		idx := 0
		// Iterate cached children and populate next level slice
		for _, children := range childrenList {
			for _, child := range children {
				next[idx] = child
				idx++
			}
		}
		// Move to next level
		current = next
	}
	return current, nil
}
