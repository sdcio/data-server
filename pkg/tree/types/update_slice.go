package types

import (
	"fmt"
	"math"
	"strings"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// UpdateSlice A slice of *Update, that defines additional helper functions.
type UpdateSlice []*Update

func (u UpdateSlice) CopyWithNewOwnerAndPrio(owner string, prio int32) UpdateSlice {
	result := u.DeepCopy()
	for _, x := range result {
		x.SetPriority(prio)
		x.SetOwner(owner)
	}
	return result
}

func (u UpdateSlice) String() string {
	sb := &strings.Builder{}
	for i, j := range u {
		sb.WriteString(fmt.Sprintf("%d - %s -> %s\n", i, j.path.String(), j.value.ToString()))
	}
	return sb.String()
}

func (u UpdateSlice) DeepCopy() UpdateSlice {
	result := make(UpdateSlice, 0, len(u))
	for _, x := range u {
		result = append(result, x.DeepCopy())
	}
	return result
}

// GetFirstPriorityValue returns the priority of the first element or math.MaxInt32 if len() is zero
func (u UpdateSlice) GetFirstPriorityValue() int32 {
	if len(u) > 0 {
		return u[0].Priority()
	}
	return int32(math.MaxInt32)
}

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

func (u UpdateSlice) ToSdcpbPathSet() *sdcpb.PathSet {
	result := &sdcpb.PathSet{}
	for _, upd := range u {
		result.AddPath(upd.path.DeepCopy())
	}
	return result
}

func Map[T any](u UpdateSlice, f func(*Update) T) []T {
	vsm := make([]T, len(u))
	for i, v := range u {
		vsm[i] = f(v)
	}
	return vsm
}
