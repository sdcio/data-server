package types

import (
	"testing"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// TestUpdateSlice_CopyWithNewOwnerAndPrio_PreservesMatchedUnionType guards PRD 003 / sync writeback:
// intent updates copied onto the "running" owner for the sync tree must keep resolved union branch
// metadata so downstream validation is not weakened.
func TestUpdateSlice_CopyWithNewOwnerAndPrio_PreservesMatchedUnionType(t *testing.T) {
	matched := &sdcpb.SchemaLeafType{TypeName: "uint32"}
	wrongFallback := &sdcpb.SchemaLeafType{TypeName: "string"}
	u := NewUpdate(nil, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_UintVal{UintVal: 42}}, 10, "intent1", 99).
		WithMatchedType(matched)

	out := UpdateSlice{u}.CopyWithNewOwnerAndPrio("running", 5)
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	got := out[0].GetUpdate()
	if got.EffectiveLeafType(wrongFallback) != matched {
		t.Errorf("EffectiveLeafType: want matched branch %v, got %v", matched, got.EffectiveLeafType(wrongFallback))
	}
	if got.Owner() != "running" || got.Priority() != 5 || got.Timestamp() != 99 {
		t.Errorf("owner/prio/ts: got owner=%q prio=%d ts=%d", got.Owner(), got.Priority(), got.Timestamp())
	}
}
