package types

import (
	"strings"
	"testing"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestEffectiveLeafType_ReturnsFallback_WhenMatchedTypeNil(t *testing.T) {
	u := NewUpdate(nil, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "foo"}}, 0, "owner", 0)
	fallback := &sdcpb.SchemaLeafType{TypeName: "string"}

	result := u.EffectiveLeafType(fallback)

	if result != fallback {
		t.Errorf("expected fallback SchemaLeafType, got %v", result)
	}
}

// EffectiveLeafType does not derive schema type from TypedValue; it returns the
// union branch type stored by WithMatchedType when present (see importer GetTVValue).
func TestEffectiveLeafType_ReturnsMatchedUnionBranchWhenSet(t *testing.T) {
	u := NewUpdate(nil, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_UintVal{UintVal: 42}}, 0, "owner", 0)
	matched := &sdcpb.SchemaLeafType{TypeName: "uint32"}
	fallback := &sdcpb.SchemaLeafType{TypeName: "uint64"}

	u.WithMatchedType(matched)
	result := u.EffectiveLeafType(fallback)

	if result != matched {
		t.Errorf("expected matched SchemaLeafType, got %v", result)
	}
}

func TestWithMatchedType_IsFluent(t *testing.T) {
	u := NewUpdate(nil, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_UintVal{UintVal: 42}}, 0, "owner", 0)
	matched := &sdcpb.SchemaLeafType{TypeName: "uint32"}

	returned := u.WithMatchedType(matched)

	if returned != u {
		t.Errorf("WithMatchedType should return the same Update for chaining")
	}
}

func TestDeepCopy_PreservesMatchedType(t *testing.T) {
	matched := &sdcpb.SchemaLeafType{TypeName: "uint32"}
	u := NewUpdate(nil, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_UintVal{UintVal: 42}}, 0, "owner", 0).
		WithMatchedType(matched)

	copied := u.DeepCopy()

	if copied.EffectiveLeafType(nil) != matched {
		t.Errorf("DeepCopy should preserve the matchedUnionType pointer")
	}
}

func TestEqual_IgnoresMatchedType(t *testing.T) {
	// Equal compares marshaled TypedValue (and owner/priority), not matchedUnionType.
	// Same wire value: one update records the resolved union branch, one does not.
	tv := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_UintVal{UintVal: 42}}
	withMatched := NewUpdate(nil, tv, 0, "owner", 0).
		WithMatchedType(&sdcpb.SchemaLeafType{TypeName: "uint32"})
	withoutMatched := NewUpdate(nil, tv, 0, "owner", 0)

	if !withMatched.Equal(withoutMatched) {
		t.Errorf("Equal should return true regardless of matchedUnionType when TypedValue bytes match")
	}
	if !withoutMatched.Equal(withMatched) {
		t.Errorf("Equal should be symmetric regardless of matchedUnionType")
	}
}

func TestEqual_falseWhenOwnerDiffers(t *testing.T) {
	tv := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "foo"}}
	a := NewUpdate(nil, tv, 0, "a", 0)
	b := NewUpdate(nil, tv, 0, "b", 0)
	if a.Equal(b) {
		t.Error("Equal should be false when intentName differs")
	}
}

func TestEqual_falseWhenPriorityDiffers(t *testing.T) {
	tv := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "foo"}}
	a := NewUpdate(nil, tv, 1, "owner", 0)
	b := NewUpdate(nil, tv, 2, "owner", 0)
	if a.Equal(b) {
		t.Error("Equal should be false when priority differs")
	}
}

func TestString_nilParentPathPlaceholder(t *testing.T) {
	u := NewUpdate(nil, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "x"}}, 0, "o", 0)
	s := u.String()
	if !strings.Contains(s, "<nil>") {
		t.Errorf("String should show <nil> path when parent nil: %q", s)
	}
}
