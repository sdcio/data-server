package types

type CacheUpdateFilter func(u *Update) bool

func CacheUpdateFilterExcludeOwner(owner string) func(u *Update) bool {
	return func(u *Update) bool {
		return u.Owner() != owner
	}
}

// ApplyCacheUpdateFilters takes a bunch of CacheUpdateFilters applies them in an AND fashion
// and returns the result.
func ApplyCacheUpdateFilters(u *Update, fs []CacheUpdateFilter) bool {
	for _, f := range fs {
		b := f(u)
		if !b {
			return false
		}
	}
	return true
}
