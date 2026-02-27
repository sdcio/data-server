package ops

import (
	"github.com/sdcio/data-server/pkg/tree/api"
)

// GetRootBasedEntryChain returns all the entries starting from the root down to the actual Entry.
// The first element of the returned slice is the root, and the last element is the actual Entry.
func GetRootBasedEntryChain(current api.Entry) []api.Entry {
	if current == nil {
		return nil
	}

	// Pre-allocate with exact size: level N means N+1 entries (including root at level 0)
	// This avoids multiple allocations as append grows the slice.
	size := current.GetLevel() + 1
	chain := make([]api.Entry, size)

	// Fill the slice backwards (from end to start) so it ends up in root-based order.
	// This eliminates the need for a separate reverse operation.
	idx := size - 1
	for current != nil {
		chain[idx] = current
		idx--
		current = current.GetParent()
	}

	return chain
}
