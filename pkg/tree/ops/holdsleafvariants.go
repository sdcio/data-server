package ops

import (
	"github.com/sdcio/data-server/pkg/tree/api"
)

func HoldsLeafVariants(e api.Entry) bool {
	return api.EntryHoldsLeafVariants(e)
}
