package ops

import "github.com/sdcio/data-server/pkg/tree/api"

// GetAncestorSchema returns the schema of the parent node if the schema is set.
// if the parent has no schema (is a key element in the tree) it will continue up the tree.
// the level of traversal is indicated via the level return value
func GetFirstAncestorWithSchema(e api.Entry) (api.Entry, int) {
	level := 0
	current := e

	// traverse up the tree until we find an ancestor with a schema
	for !current.IsRoot() {
		current = current.GetParent()
		level++
		if current.GetSchema() != nil {
			return current, level
		}
	}

	// reached root without finding a schema
	return nil, 0
}
