package tree

import (
	"strings"
	"testing"

	"github.com/sdcio/data-server/pkg/cache"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

func Test_Entry(t *testing.T) {

	desc, err := proto.Marshal(&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "MyDescription"}})
	if err != nil {
		t.Error(err)
	}

	u1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "9", "description"}, desc, int32(100), "me", int64(9999999))

	u2 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, desc, int32(99), "me", int64(444))
	u3 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, desc, int32(98), "me", int64(88))

	root := NewRootEntry()

	for _, u := range []*cache.Update{u1, u2, u3} {
		err = root.AddCacheUpdateRecursive(u)
		if err != nil {
			t.Error(err)
		}
	}

	r := []string{}
	r = root.StringIndent(r)
	t.Log(strings.Join(r, "\n"))
}
