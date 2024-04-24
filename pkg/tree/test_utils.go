package tree

import (
	"fmt"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sdcio/data-server/pkg/cache"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

// diffCacheUpdates takes two []*cache.Update and compares the diff
func diffCacheUpdates(a, b []*cache.Update) string {
	return cmp.Diff(cacheUpdateSliceToStringSlice(a), cacheUpdateSliceToStringSlice(b))
}

// cacheUpdateSliceToStringSlice converts a []*cache.Update to []string
func cacheUpdateSliceToStringSlice(s []*cache.Update) []string {
	result := make([]string, 0, len(s))
	for _, e := range s {
		result = append(result, fmt.Sprintf("%v", e))
	}
	// sort the result lexically
	slices.Sort(result)
	return result
}

// getStringTvProto takes a string and returns the sdcpb.TypedValue for it in proto encoding as []byte
func getStringTvProto(t *testing.T, s string) []byte {
	result, err := proto.Marshal(&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: s}})
	if err != nil {
		t.Error(err)
	}
	return result
}
