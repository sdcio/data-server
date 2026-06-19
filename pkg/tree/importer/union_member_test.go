package importer_test

import (
	"testing"

	"github.com/sdcio/data-server/pkg/tree/importer"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestInferUnionMemberFromTypedValue_TwoStringBranchesRFCFirstMember(t *testing.T) {
	tv := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "hello"}}
	first := &sdcpb.SchemaLeafType{Type: "string"}
	second := &sdcpb.SchemaLeafType{Type: "string"}
	decl := &sdcpb.SchemaLeafType{
		Type:       "union",
		UnionTypes: []*sdcpb.SchemaLeafType{first, second},
	}
	m := importer.InferUnionMemberFromTypedValue(tv, decl)
	if m != first {
		t.Fatalf("RFC 7950 §9.12 first matching member: want first branch pointer, got %v (first=%p second=%p)", m, first, second)
	}
}

func TestInferUnionMemberFromTypedValue_NonUnion(t *testing.T) {
	tv := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "x"}}
	decl := &sdcpb.SchemaLeafType{Type: "string"}
	if m := importer.InferUnionMemberFromTypedValue(tv, decl); m != nil {
		t.Fatalf("non-union declared type: want nil, got %v", m)
	}
}
