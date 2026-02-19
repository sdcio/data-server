package opsinterface

import (
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

type Entry interface {
	GetChilds(types.DescendMethod) api.EntryMap
	GetLeafVariants() *api.LeafVariants
}
