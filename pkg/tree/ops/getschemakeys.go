package ops

import (
	"slices"
	"sort"

	"github.com/sdcio/data-server/pkg/tree/api"
)

// GetSchemaKeys returns list key leaf names in YANG schema declaration order
// (the order of the bound schema's `key` statement).
func GetSchemaKeys(e api.Entry) []string {
	if e.GetSchema() != nil {
		// if the schema is a container schema, we need to process the aggregation logic
		if contschema := e.GetSchema().GetContainer(); contschema != nil {
			// if the level equals the amount of keys defined, we're at the right level, where the
			// actual elements start (not in a key level within the tree)
			keys := make([]string, 0, len(contschema.GetKeys()))
			for _, k := range contschema.GetKeys() {
				keys = append(keys, k.Name)
			}
			return keys
		}
	}
	return nil
}

// GetSchemaKeysAlphabeticalOrder returns list key leaf names sorted alphabetically,
// matching tree key level order.
func GetSchemaKeysAlphabeticalOrder(e api.Entry) []string {
	keys := GetSchemaKeys(e)
	if len(keys) == 0 {
		return nil
	}
	keys = slices.Clone(keys)
	sort.Strings(keys)
	return keys
}
