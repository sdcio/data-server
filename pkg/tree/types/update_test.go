package types

import (
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

func TestEffectiveLeafType_ReturnsMatchedType_WhenSet(t *testing.T) {
	u := NewUpdate(nil, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "foo"}}, 0, "owner", 0)
	matched := &sdcpb.SchemaLeafType{TypeName: "uint32"}
	fallback := &sdcpb.SchemaLeafType{TypeName: "string"}

	u.WithMatchedType(matched)
	result := u.EffectiveLeafType(fallback)

	if result != matched {
		t.Errorf("expected matched SchemaLeafType, got %v", result)
	}
}

func TestWithMatchedType_IsFluent(t *testing.T) {
	u := NewUpdate(nil, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "foo"}}, 0, "owner", 0)
	matched := &sdcpb.SchemaLeafType{TypeName: "uint32"}

	returned := u.WithMatchedType(matched)

	if returned != u {
		t.Errorf("WithMatchedType should return the same Update for chaining")
	}
}

func TestDeepCopy_PreservesMatchedType(t *testing.T) {
	matched := &sdcpb.SchemaLeafType{TypeName: "uint32"}
	u := NewUpdate(nil, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "foo"}}, 0, "owner", 0).
		WithMatchedType(matched)

	copied := u.DeepCopy()

	if copied.EffectiveLeafType(nil) != matched {
		t.Errorf("DeepCopy should preserve the matchedUnionType pointer")
	}
}

func TestEqual_IgnoresMatchedType(t *testing.T) {
	tv := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "foo"}}
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
