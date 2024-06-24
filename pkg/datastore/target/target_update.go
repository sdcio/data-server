package target

import (
	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/tree"
)

type TargetUpdate interface {
	GetUpdate() *cache.Update
	GetTreeEntry() tree.Entry
}
