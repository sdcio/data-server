package api

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type LeafVariantSlice []*LeafEntry

func (lvs LeafVariantSlice) ToUpdateSlice() types.UpdateSlice {
	result := make([]*types.Update, 0, len(lvs))
	for _, x := range lvs {
		result = append(result, x.Update)
	}
	return result
}

func (lvs LeafVariantSlice) ToPathAndUpdateSlice() []*types.PathAndUpdate {
	result := make([]*types.PathAndUpdate, 0, len(lvs))
	for _, x := range lvs {
		result = append(result, types.NewPathAndUpdate(x.Path(), x.GetUpdate()))
	}
	return result
}

func (lvs LeafVariantSlice) ToSdcpbUpdateSlice() []*sdcpb.Update {
	result := make([]*sdcpb.Update, 0, len(lvs))
	for _, x := range lvs {
		result = append(result, x.ToSdcpbUpdate())
	}
	return result
}

// Equal checks equality of the LeafVariantSlice with the other LeafVariantSlice
func (lvs LeafVariantSlice) Equal(otherLvs LeafVariantSlice) (bool, error) {

	// check for equal length
	if len(lvs) != len(otherLvs) {
		return false, fmt.Errorf("LeafVariantSlices differ in length %d vs. %d", len(lvs), len(otherLvs))
	}

	// sort lvs
	slices.SortFunc(lvs, func(le1, le2 *LeafEntry) int {
		return le1.Compare(le2)
	})
	// sort otherLvs
	slices.SortFunc(lvs, func(le1, le2 *LeafEntry) int {
		return le1.Compare(le2)
	})

	// compare one by one
	for idx, le1 := range lvs {
		equal := le1.Equal(otherLvs[idx])
		if !equal {
			return false, nil
		}
	}
	return true, nil
}

func (lvs LeafVariantSlice) String() string {
	sb := strings.Builder{}
	first := true
	sep := ""
	for _, item := range lvs {
		sb.WriteString(sep)
		sb.WriteString(item.Path().ToXPath(false))
		sb.WriteString(" -> ")
		sb.WriteString(item.String())
		if first {
			sep = "\n"
			first = false
		}
	}
	return sb.String()
}
