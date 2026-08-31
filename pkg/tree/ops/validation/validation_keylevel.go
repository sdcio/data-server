package validation

import (
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

// descendKeyLevels steps down the tree through the given number of key levels,
// so that fn is only ever invoked on resolved list instances, never on the
// list's key-level node itself (the node that carries the list's schema but
// whose children are the keyed instances, not the instance's own siblings).
//
// A list with N keys has N key levels between its schema-bearing node and its
// instances; level is decremented by one per level until it reaches 0, at
// which point e is an instance and fn is invoked on it.
func descendKeyLevels(e api.Entry, level int, fn func(instance api.Entry)) {
	if e.ShouldDelete() {
		return
	}
	if level > 0 {
		for _, c := range e.GetChilds(types.DescendMethodActiveChilds) {
			descendKeyLevels(c, level-1, fn)
		}
		return
	}
	fn(e)
}
