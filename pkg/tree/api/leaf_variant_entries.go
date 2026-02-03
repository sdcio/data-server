package api

import "github.com/sdcio/data-server/pkg/tree/types"

type LeafVariantEntries interface {
	MarkOwnerForDeletion(owner string, onlyIntended bool) *LeafEntry
	ResetFlags(deleteFlag bool, newFlag bool, updatedFlag bool) int
	GetHighestPrecedence(onlyNewOrUpdated bool, includeDefaults bool, includeExplicitDeletes bool) *LeafEntry
	GetRunning() *LeafEntry
	DeleteByOwner(owner string) *LeafEntry
	AddExplicitDeleteEntry(owner string, priority int32) *LeafEntry
	GetByOwner(owner string) *LeafEntry
	RemoveDeletedByOwner(owner string) *LeafEntry
	Add(l *LeafEntry)
	AddWithStats(l *LeafEntry, stats *types.ImportStats)
	Length() int
}
