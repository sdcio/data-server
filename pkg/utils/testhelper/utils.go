package testhelper

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sdcio/data-server/pkg/cache"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

const (
	pathSep = "/"
)

// diffCacheUpdates takes two []*cache.Update and compares the diff
func DiffCacheUpdates(a, b []*cache.Update) string {
	return cmp.Diff(CacheUpdateSliceToStringSlice(a), CacheUpdateSliceToStringSlice(b))
}

// CacheUpdateSliceToStringSlice converts a []*cache.Update to []string
func CacheUpdateSliceToStringSlice(s []*cache.Update) []string {
	result := make([]string, 0, len(s))
	for _, e := range s {
		result = append(result, fmt.Sprintf("%v", e))
	}
	// sort the result lexically
	slices.Sort(result)
	return result
}

// GetStringTvProto takes a string and returns the sdcpb.TypedValue for it in proto encoding as []byte
func GetStringTvProto(t *testing.T, s string) []byte {
	result, err := proto.Marshal(&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: s}})
	if err != nil {
		t.Error(err)
	}
	return result
}

// PathMapIndex calculates a common map index for string slice based paths
func PathMapIndex(elems []string) string {
	return strings.Join(elems, pathSep)
}

// DiffStringSlice compares two string slices returning the
func DiffStringSlice(s1, s2 []string, forceNoSideEffect bool) string {
	var tmp []string
	// to avoid side effects we copy the slices before sorting them
	if forceNoSideEffect {
		copy(s1, tmp)
		s1 = tmp
		copy(s2, tmp)
		s2 = tmp
	}

	slices.Sort(s1)
	slices.Sort(s2)
	return cmp.Diff(s1, s2)
}

func DiffDoubleStringPathSlice(s1, s2 [][]string) string {
	s1StringSlice := make([]string, 0, len(s1))
	s2StringSlice := make([]string, 0, len(s2))

	y := []struct {
		Double [][]string
		Single []string
	}{
		{
			Double: s1,
			Single: s1StringSlice,
		},
		{
			Double: s2,
			Single: s2StringSlice,
		},
	}

	for _, x := range y {
		for _, entry := range x.Double {
			x.Single = append(x.Single, PathMapIndex(entry))
		}
	}
	return DiffStringSlice(y[0].Single, y[1].Single, false)
}
