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

	got := updates[0].GetValue().GetStringVal()
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

	got := updates[0].GetValue().GetUintVal()
	if got != ^uint64(0) {
		t.Fatalf("expected max uint64 %d, got %d", ^uint64(0), got)
	}
}

func TestGetChildRootUsesModuleQualifiedSchemaLookup(t *testing.T) {
	var gotOrigin, gotLocal string
	scb := &testSchemaClientBound{
		getSchemaPathFn: func(_ context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			gotOrigin = path.GetOrigin()
			if len(path.GetElem()) > 0 {
				gotLocal = path.GetElem()[0].GetName()
			}
			return &sdcpb.GetSchemaResponse{
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Container{
						Container: &sdcpb.ContainerSchema{Name: "router"},
					},
				},
			}, nil
		},
	}
	rootCS := &sdcpb.SchemaElem_Container{
		Container: &sdcpb.ContainerSchema{Name: "__root__", Children: []string{"router"}},
	}

	child, ok := getChild(context.Background(), "mod-a:router", rootCS, scb)
	if !ok || child != "router" {
		t.Fatalf("getChild: ok=%v child=%v", ok, child)
	}
	if gotOrigin != "mod-a" || gotLocal != "router" {
		t.Fatalf("schema lookup path origin=%q local=%q want mod-a / router", gotOrigin, gotLocal)
	}
}

func TestGetChildRootReturnsFieldAndLeaflist(t *testing.T) {
	scb := &testSchemaClientBound{
		getSchemaPathFn: func(_ context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			name := ""
			if len(path.GetElem()) > 0 {
				name = path.GetElem()[0].GetName()
			}
			switch name {
			case "patterntest":
				return &sdcpb.GetSchemaResponse{
					Schema: &sdcpb.SchemaElem{
						Schema: &sdcpb.SchemaElem_Field{
							Field: &sdcpb.LeafSchema{Name: "patterntest", ModuleName: "sdcio_model"},
						},
					},
				}, nil
			case "tags":
				return &sdcpb.GetSchemaResponse{
					Schema: &sdcpb.SchemaElem{
						Schema: &sdcpb.SchemaElem_Leaflist{
							Leaflist: &sdcpb.LeafListSchema{Name: "tags", ModuleName: "sdcio_model"},
						},
					},
				}, nil
			default:
				t.Fatalf("unexpected lookup %v", path)
				return nil, nil
			}
		},
	}
	rootCS := &sdcpb.SchemaElem_Container{
		Container: &sdcpb.ContainerSchema{Name: "__root__"},
	}

	child, ok := getChild(context.Background(), "patterntest", rootCS, scb)
	if !ok {
		t.Fatal("getChild(patterntest) not found")
	}
	field, ok := child.(*sdcpb.LeafSchema)
	if !ok || field.GetName() != "patterntest" {
		t.Fatalf("patterntest: got %#v", child)
	}

	child, ok = getChild(context.Background(), "sdcio_model:tags", rootCS, scb)
	if !ok {
		t.Fatal("getChild(sdcio_model:tags) not found")
	}
	lfl, ok := child.(*sdcpb.LeafListSchema)
	if !ok || lfl.GetName() != "tags" {
		t.Fatalf("tags: got %#v", child)
	}
}

func TestExpandContainerValueRootChildSetsOriginOnUpdatePath(t *testing.T) {
	const wantOrigin = "mod-a"
	scb := &testSchemaClientBound{
		getSchemaPathFn: func(_ context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			elems := path.GetElem()
			if len(elems) == 2 && elems[1].GetName() == "enabled" {
				return &sdcpb.GetSchemaResponse{
					Schema: &sdcpb.SchemaElem{
						Schema: &sdcpb.SchemaElem_Field{
							Field: &sdcpb.LeafSchema{
								Name: "enabled",
								Type: &sdcpb.SchemaLeafType{Type: "boolean", TypeName: "boolean"},
							},
						},
					},
				}, nil
			}
			if path.GetOrigin() != wantOrigin {
				t.Fatalf("child schema lookup origin %q want %q", path.GetOrigin(), wantOrigin)
			}
			return &sdcpb.GetSchemaResponse{
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Container{
						Container: &sdcpb.ContainerSchema{
							Name:   "router",
							Fields: []*sdcpb.LeafSchema{{Name: "enabled", Type: &sdcpb.SchemaLeafType{Type: "boolean"}}},
						},
					},
				},
			}, nil
		},
	}
	converter := NewConverter(scb)
	rootCS := &sdcpb.SchemaElem_Container{
		Container: &sdcpb.ContainerSchema{Name: "__root__", Children: []string{"router"}},
	}
	upds, err := converter.ExpandContainerValue(context.Background(), &sdcpb.Path{}, map[string]any{
		"mod-a:router": map[string]any{"enabled": true},
	}, rootCS)
	if err != nil {
		t.Fatalf("ExpandContainerValue: %v", err)
	}
	if len(upds) != 1 {
		t.Fatalf("expected 1 update, got %d", len(upds))
	}
	p := upds[0].GetPath()
	if p.GetOrigin() != wantOrigin {
		t.Fatalf("update path origin %q want %q", p.GetOrigin(), wantOrigin)
	}
	elems := p.GetElem()
	if len(elems) != 2 || elems[0].GetName() != "router" || elems[1].GetName() != "enabled" {
		t.Fatalf("unexpected path elems: %v", elems)
	}
}

func TestPathForChildContainerNestedModulePrefix(t *testing.T) {
	parent := &sdcpb.Path{
		Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("parent", nil)},
	}
	cs := &sdcpb.SchemaElem_Container{Container: &sdcpb.ContainerSchema{Name: "parent"}}
	np := pathForChildContainer(parent, cs, "mod-a:child", "child")
	last := np.Elem[len(np.Elem)-1]
	if last.GetName() != "mod-a:child" {
		t.Fatalf("nested child path elem %q want mod-a:child", last.GetName())
	}
	if np.GetOrigin() != "" {
		t.Fatalf("nested path must not set origin, got %q", np.GetOrigin())
	}
}
