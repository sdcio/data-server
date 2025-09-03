package tree

import "github.com/sdcio/data-server/pkg/tree/types"

type LeafEntryFilter func(*LeafEntry) bool

// FilterNonDeleted Accepts all Entries that are not marked as deleted
func FilterNonDeleted(l *LeafEntry) bool {
	return !l.GetDeleteFlag()
}

// FilterNonDeletedButNewOrUpdated Accepts all Entries that are New or Updated and not Deleted.
func FilterNonDeletedButNewOrUpdated(l *LeafEntry) bool {
	return !l.GetDeleteFlag() && (l.GetUpdateFlag() || l.GetNewFlag())
}

// FilterDeleted Accepts all Entries that are marked as deleted
func FilterDeleted(l *LeafEntry) bool {
	return l.GetDeleteFlag()
}

func FilterDeletedNotExplicitDelete(l *LeafEntry) bool {
	return l.GetDeleteFlag() && !l.GetExplicitDeleteFlag()
}

// FilterNew Accepts New LeafEntries
func FilterNew(l *LeafEntry) bool {
	return l.GetNewFlag()
}

// FilterUpdated Accepts all entries that are updates
func FilterUpdated(l *LeafEntry) bool {
	return l.GetUpdateFlag()
}

// Unfiltered accepts all entries without any filtering
func Unfiltered(l *LeafEntry) bool {
	return true
}

// LeafEntriesToCacheUpdates
func LeafEntriesToUpdates(l []*LeafEntry) []*types.Update {
	result := make([]*types.Update, 0, len(l))
	for _, e := range l {
		result = append(result, e.Update)
	}
	return result
}
