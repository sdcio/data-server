package tree

import "github.com/sdcio/data-server/pkg/cache"

type CacheUpdateFilter func(u *cache.Update) bool

func CacheUpdateFilterExcludeOwner(owner string) func(u *cache.Update) bool {
	return func(u *cache.Update) bool {
		return u.Owner() != owner
	}
}

// ApplyCacheUpdateFilters takes a bunch of CacheUpdateFilters applies them in an AND fashion
// and returns the result.
func ApplyCacheUpdateFilters(u *cache.Update, fs []CacheUpdateFilter) bool {
	for _, f := range fs {
		b := f(u)
		if !b {
			return false
		}
	}
	return true
}
