package ops

import "github.com/sdcio/data-server/pkg/tree/api"

// GetSchemaKeys checks for the schema of the entry, and returns the defined keys
func GetSchemaKeys(e api.Entry) []string {
	if e.GetSchema() != nil {
		// if the schema is a container schema, we need to process the aggregation logic
		if contschema := e.GetSchema().GetContainer(); contschema != nil {
			// if the level equals the amount of keys defined, we're at the right level, where the
			// actual elements start (not in a key level within the tree)
			var keys []string
			for _, k := range contschema.GetKeys() {
				keys = append(keys, k.Name)
			}
			return keys
		}
	}
	return nil
}
