package ops

import "github.com/sdcio/data-server/pkg/tree/api"

// GetRoot returns the Trees Root Entry
func GetRoot(e api.Entry) api.Entry {
	for !e.IsRoot() {
		e = e.GetParent()
	}
	return e
}
