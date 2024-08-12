package tree

import (
	"math"

	"github.com/sdcio/data-server/pkg/cache"
)

// UpdateSlice A slice of *cache.Update, that defines additional helper functions.
type UpdateSlice []*cache.Update

// GetHighesPriorityValue returns the highes priority value of all the containing Updates
func (u UpdateSlice) GetLowestPriorityValue(filters []CacheUpdateFilter) int32 {
	result := int32(math.MaxInt32)
	for _, entry := range u {
		if entry.Priority() < result && ApplyCacheUpdateFilters(entry, filters) {
			result = entry.Priority()
		}
	}
	return result
}

func Map[T any](u UpdateSlice, f func(*cache.Update) T) []T {
	vsm := make([]T, len(u))
	for i, v := range u {
		vsm[i] = f(v)
	}
	return vsm
}
