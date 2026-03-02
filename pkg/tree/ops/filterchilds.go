package ops

import (
	"fmt"
	"sort"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

// FilterChilds returns the child entries (skipping the key entries in the tree) that
// match the given keys. The keys do not need to match all levels of keys, in which case the
// key level is considered a wildcard match (*)
func FilterChilds(s api.Entry, keys map[string]string) ([]api.Entry, error) {
	if s.GetSchema() == nil {
		return nil, fmt.Errorf("error non schema level %s", s.SdcpbPath().ToXPath(false))
	}

	// Retrieve and sort schema keys to maintain insertion order
	schemaKeys := GetSchemaKeys(s)
	sort.Strings(schemaKeys)

	// Start with the root entry
	currentEntries := []api.Entry{s}

	// Iterate through each key level, filtering or expanding entries
	for _, key := range schemaKeys {
		var nextEntries []api.Entry

		if keyVal, exists := keys[key]; exists {
			// Filter: find children matching the specific key value
			for _, entry := range currentEntries {
				children := entry.GetChilds(types.DescendMethodAll)
				if matchEntry, found := children[keyVal]; found {
					nextEntries = append(nextEntries, matchEntry)
				}
			}
		} else {
			// Wildcard: collect all children
			for _, entry := range currentEntries {
				children := entry.GetChilds(types.DescendMethodAll)
				for _, child := range children {
					nextEntries = append(nextEntries, child)
				}
			}
		}

		currentEntries = nextEntries
	}

	return currentEntries, nil
}
