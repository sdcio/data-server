package api_test

import (
	"context"
	"testing"

	"github.com/sdcio/data-server/pkg/tree/api"
	treeconsts "github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// pathOnlyParent satisfies api.Entry through the embedded (nil) interface; the code under
// test only calls SdcpbPath on the parent.
type pathOnlyParent struct{ api.Entry }

func (pathOnlyParent) SdcpbPath() *sdcpb.Path { return &sdcpb.Path{} }

// Explicit deletes (sweepers) add leaf entries with an empty TypedValue. GetDeviations used to
// compare that empty value with TypedValue.Equal, which panics on the unset oneof.
func TestLeafVariants_GetDeviations_ExplicitDelete(t *testing.T) {
	str := func(s string) *sdcpb.TypedValue {
		return &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: s}}
	}
	running := treeconsts.RunningIntentName
	runningPrio := treeconsts.RunningValuesPrio

	type entry struct {
		owner string
		prio  int32
		value *sdcpb.TypedValue // nil: explicit delete
	}
	type dev struct {
		owner  string
		reason types.DeviationReason
	}
	tests := []struct {
		name    string
		entries []entry
		want    []dev
	}{
		{
			name:    "sweeper loses to intent, device matches the intent",
			entries: []entry{{"intent", 10, str("a")}, {"sweeper", 20, nil}, {running, runningPrio, str("a")}},
			want:    nil,
		},
		{
			name:    "sweeper wins, value still on the device",
			entries: []entry{{"sweeper", 20, nil}, {running, runningPrio, str("x")}},
			want:    []dev{{"sweeper", types.DeviationReasonNotApplied}},
		},
		{
			name:    "sweeper wins, leaf absent",
			entries: []entry{{"sweeper", 20, nil}},
			want:    nil,
		},
		{
			name:    "sweeper overrules a lower-precedence intent",
			entries: []entry{{"sweeper", 20, nil}, {"low", 30, str("b")}, {running, runningPrio, str("b")}},
			want:    []dev{{"low", types.DeviationReasonOverruled}, {"sweeper", types.DeviationReasonNotApplied}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := pathOnlyParent{}
			lv := api.NewLeafVariants(nil, parent)
			for _, e := range tt.entries {
				if e.value == nil {
					lv.AddExplicitDeleteEntry(e.owner, e.prio)
					continue
				}
				lv.Add(api.NewLeafEntry(types.NewUpdate(parent, e.value, e.prio, e.owner, 0), types.NewUpdateInsertFlags(), parent))
			}

			ch := make(chan *types.DeviationEntry, 10)
			lv.GetDeviations(context.Background(), ch, true)
			close(ch)

			var got []dev
			for de := range ch {
				got = append(got, dev{de.IntentName(), de.Reason()})
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d deviations %v, want %d %v", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("deviation %d: got %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
