package utils

import (
	"context"
	"testing"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type testSchemaClientBound struct {
	getSchemaPathFn func(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error)
}

func (t *testSchemaClientBound) GetSchemaSdcpbPath(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
	return t.getSchemaPathFn(ctx, path)
}

func (t *testSchemaClientBound) GetSchemaElements(context.Context, *sdcpb.Path, chan struct{}) (chan *sdcpb.GetSchemaResponse, error) {
	return nil, nil
}

func TestExpandUpdateFieldJSONStringNormalizesQuotes(t *testing.T) {
	converter := NewConverter(&testSchemaClientBound{
		getSchemaPathFn: func(context.Context, *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			return &sdcpb.GetSchemaResponse{
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Field{
						Field: &sdcpb.LeafSchema{
							Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"},
						},
					},
				},
			}, nil
		},
	})

	updates, err := converter.ExpandUpdate(context.Background(), &sdcpb.Update{
		Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "root"}, {Name: "leaf"}}},
		Value: &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(`"aes128-cbc"`)},
		},
	})
	if err != nil {
		t.Fatalf("ExpandUpdate returned error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}

	got := updates[0].Update.GetValue().GetStringVal()
	if got != "aes128-cbc" {
		t.Fatalf("expected unquoted value %q, got %q", "aes128-cbc", got)
	}
}

func TestExpandUpdateFieldJSONUint64PreservesPrecision(t *testing.T) {
	converter := NewConverter(&testSchemaClientBound{
		getSchemaPathFn: func(context.Context, *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			return &sdcpb.GetSchemaResponse{
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Field{
						Field: &sdcpb.LeafSchema{
							Type: &sdcpb.SchemaLeafType{Type: "uint64", TypeName: "uint64"},
						},
					},
				},
			}, nil
		},
	})

	updates, err := converter.ExpandUpdate(context.Background(), &sdcpb.Update{
		Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "root"}, {Name: "leaf"}}},
		Value: &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(`18446744073709551615`)},
		},
	})
	if err != nil {
		t.Fatalf("ExpandUpdate returned error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}

	got := updates[0].Update.GetValue().GetUintVal()
	if got != ^uint64(0) {
		t.Fatalf("expected max uint64 %d, got %d", ^uint64(0), got)
	}
}

func TestExpandUpdate_fieldJSONUnion_setsMatchedUnionBranchOnExpandedUpdate(t *testing.T) {
	uint32Branch := &sdcpb.SchemaLeafType{Type: "uint32"}
	stringBranch := &sdcpb.SchemaLeafType{Type: "string"}
	unionDecl := &sdcpb.SchemaLeafType{
		Type:       "union",
		UnionTypes: []*sdcpb.SchemaLeafType{uint32Branch, stringBranch},
	}
	converter := NewConverter(&testSchemaClientBound{
		getSchemaPathFn: func(context.Context, *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			return &sdcpb.GetSchemaResponse{
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Field{
						Field: &sdcpb.LeafSchema{Type: unionDecl},
					},
				},
			}, nil
		},
	})

	expanded, err := converter.ExpandUpdate(context.Background(), &sdcpb.Update{
		Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "system"}, {Name: "u"}}},
		Value: &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(`42`)},
		},
	})
	if err != nil {
		t.Fatalf("ExpandUpdate: %v", err)
	}
	if len(expanded) != 1 {
		t.Fatalf("expected 1 expanded update, got %d", len(expanded))
	}
	if expanded[0].MatchedUnionType == nil || expanded[0].MatchedUnionType.Type != "uint32" {
		t.Fatalf("MatchedUnionType want uint32 branch, got %v", expanded[0].MatchedUnionType)
	}
	if expanded[0].Update.GetValue().GetUintVal() != 42 {
		t.Fatalf("TypedValue: want uint 42, got %v", expanded[0].Update.GetValue())
	}
}

// TestExpandUpdate_containerJSONUnionLeafList_leavesMatchedUnionTypeNil documents
// Phase 1: union-typed leaf-lists expand to a correct LeaflistVal but do not attach
// MatchedUnionType on the enclosing update (per docs/prd/union-member-resolution-
// validation/issues/007-DECISION.md).
func TestExpandUpdate_containerJSONUnionLeafList_leavesMatchedUnionTypeNil(t *testing.T) {
	uint32Branch := &sdcpb.SchemaLeafType{Type: "uint32", TypeName: "uint32"}
	stringBranch := &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}
	unionDecl := &sdcpb.SchemaLeafType{
		Type:       "union",
		UnionTypes: []*sdcpb.SchemaLeafType{uint32Branch, stringBranch},
	}
	containerSchema := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Container{
			Container: &sdcpb.ContainerSchema{
				Name: "sys",
				Leaflists: []*sdcpb.LeafListSchema{
					{Name: "items", Type: unionDecl},
				},
			},
		},
	}
	converter := NewConverter(&testSchemaClientBound{
		getSchemaPathFn: func(context.Context, *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			return &sdcpb.GetSchemaResponse{Schema: containerSchema}, nil
		},
	})

	expanded, err := converter.ExpandUpdate(context.Background(), &sdcpb.Update{
		Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "sys"}}},
		Value: &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(`{"items":[42,43]}`)},
		},
	})
	if err != nil {
		t.Fatalf("ExpandUpdate: %v", err)
	}
	if len(expanded) != 1 {
		t.Fatalf("expected 1 expanded update, got %d", len(expanded))
	}
	if expanded[0].MatchedUnionType != nil {
		t.Fatalf("MatchedUnionType must be nil for union leaf-list in Phase 1, got %#v", expanded[0].MatchedUnionType)
	}
	elems := expanded[0].Update.GetValue().GetLeaflistVal().GetElement()
	if len(elems) != 2 || elems[0].GetUintVal() != 42 || elems[1].GetUintVal() != 43 {
		t.Fatalf("LeaflistVal elements: want [42,43], got %#v", elems)
	}
}
