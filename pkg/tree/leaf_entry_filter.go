package tree

import (
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

type LeafEntryFilter func(*api.LeafEntry) bool

// FilterNonDeleted Accepts all Entries that are not marked as deleted
func FilterNonDeleted(l *api.LeafEntry) bool {
	return !l.GetDeleteFlag()
}

// FilterNonDeletedButNewOrUpdated Accepts all Entries that are New or Updated and not Deleted.
func FilterNonDeletedButNewOrUpdated(l *api.LeafEntry) bool {
	return !l.GetDeleteFlag() && (l.GetUpdateFlag() || l.GetNewFlag())
}

// FilterDeleted Accepts all Entries that are marked as deleted
func FilterDeleted(l *api.LeafEntry) bool {
	return l.GetDeleteFlag()
}

func FilterDeletedNotExplicitDelete(l *api.LeafEntry) bool {
	return l.GetDeleteFlag() && !l.GetExplicitDeleteFlag()
}

// FilterNew Accepts New LeafEntries
func FilterNew(l *api.LeafEntry) bool {
	return l.GetNewFlag()
}

// FilterUpdated Accepts all entries that are updates
func FilterUpdated(l *api.LeafEntry) bool {
	return l.GetUpdateFlag()
}

// Unfiltered accepts all entries without any filtering
func Unfiltered(l *api.LeafEntry) bool {
	return true
}

// LeafEntriesToCacheUpdates
func LeafEntriesToUpdates(l []*api.LeafEntry) []*types.Update {
	result := make([]*types.Update, 0, len(l))
	for _, e := range l {
		result = append(result, e.Update)
	}
	return result
}
