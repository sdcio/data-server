package utils

import (
	"context"
	"fmt"
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

// TestExpandUpdateContainerUnwrapsRFC7951SelfReference reproduces a GET
// response shape seen from SONiC translib: a top-level container's value is
// wrapped in its own module-qualified name, e.g.
// {"sonic-srv6:sonic-srv6": {"enabled": "true"}} for a GET on
// .../sonic-srv6, instead of the bare {"enabled": "true"}. This must be
// unwrapped rather than treated as an unknown child object.
func TestExpandUpdateContainerUnwrapsRFC7951SelfReference(t *testing.T) {
	containerSchema := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Container{
			Container: &sdcpb.ContainerSchema{
				Name:       "sonic-srv6",
				ModuleName: "sonic-srv6",
				Fields: []*sdcpb.LeafSchema{
					{Name: "enabled", ModuleName: "sonic-srv6", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
				},
			},
		},
	}
	fieldSchema := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{
			Field: &sdcpb.LeafSchema{
				Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"},
			},
		},
	}

	converter := NewConverter(&testSchemaClientBound{
		getSchemaPathFn: func(_ context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			elems := path.GetElem()
			if len(elems) > 0 && elems[len(elems)-1].GetName() == "enabled" {
				return &sdcpb.GetSchemaResponse{Schema: fieldSchema}, nil
			}
			return &sdcpb.GetSchemaResponse{Schema: containerSchema}, nil
		},
	})

	updates, err := converter.ExpandUpdate(context.Background(), &sdcpb.Update{
		Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "sonic-srv6"}}},
		Value: &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"sonic-srv6:sonic-srv6":{"enabled":"true"}}`)},
		},
	})
	if err != nil {
		t.Fatalf("ExpandUpdate returned error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}

	gotPath := updates[0].GetPath()
	wantElems := []string{"sonic-srv6", "enabled"}
	if len(gotPath.GetElem()) != len(wantElems) {
		t.Fatalf("expected path elems %v, got %v", wantElems, gotPath.GetElem())
	}
	for i, e := range gotPath.GetElem() {
		if e.GetName() != wantElems[i] {
			t.Fatalf("expected path elems %v, got %v", wantElems, gotPath.GetElem())
		}
	}

	got := updates[0].GetValue().GetStringVal()
	if got != "true" {
		t.Fatalf("expected value %q, got %q", "true", got)
	}
}

// TestExpandUpdateContainerDoesNotUnwrapLegitimateChildSharingTheContainersName
// guards against a false-positive unwrap: some real (non-SONiC) schemas have
// a container whose single currently-populated key happens to render as
// "<moduleName>:<containerName>" purely because a genuine child field is
// named identically to the parent container itself (a real, if unusual,
// schema shape) — not because the target self-wrapped its response. Since
// that key resolves as an actual schema member via getItem, it must be
// processed as a normal field, not recursively unwrapped as a self-reference
// (which would silently drop a path segment for every non-SONiC target that
// happens to hit this naming coincidence).
func TestExpandUpdateContainerDoesNotUnwrapLegitimateChildSharingTheContainersName(t *testing.T) {
	containerSchema := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Container{
			Container: &sdcpb.ContainerSchema{
				Name:       "foo",
				ModuleName: "m",
				Fields: []*sdcpb.LeafSchema{
					{Name: "foo", ModuleName: "m", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
				},
			},
		},
	}
	fieldSchema := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{
			Field: &sdcpb.LeafSchema{
				Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"},
			},
		},
	}

	converter := NewConverter(&testSchemaClientBound{
		getSchemaPathFn: func(_ context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			elems := path.GetElem()
			if len(elems) > 1 && elems[len(elems)-1].GetName() == "foo" {
				return &sdcpb.GetSchemaResponse{Schema: fieldSchema}, nil
			}
			return &sdcpb.GetSchemaResponse{Schema: containerSchema}, nil
		},
	})

	updates, err := converter.ExpandUpdate(context.Background(), &sdcpb.Update{
		Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "foo"}}},
		Value: &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"m:foo":"bar"}`)},
		},
	})
	if err != nil {
		t.Fatalf("ExpandUpdate returned error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}

	gotPath := updates[0].GetPath()
	wantElems := []string{"foo", "foo"}
	if len(gotPath.GetElem()) != len(wantElems) {
		t.Fatalf("expected path elems %v, got %v", wantElems, gotPath.GetElem())
	}
	for i, e := range gotPath.GetElem() {
		if e.GetName() != wantElems[i] {
			t.Fatalf("expected path elems %v, got %v", wantElems, gotPath.GetElem())
		}
	}
	got := updates[0].GetValue().GetStringVal()
	if got != "bar" {
		t.Fatalf("expected value %q, got %q", "bar", got)
	}
}

// TestExpandUpdatesBatchOfSelfWrappedContainers mirrors a single GET sync
// that requests several distinct top-level SONiC containers in one
// GetDataRequest (one gnmi.Update per path, all in the same batch). Each
// Update must be unwrapped and resolved independently, without any
// cross-talk between paths.
func TestExpandUpdatesBatchOfSelfWrappedContainers(t *testing.T) {
	makeContainerSchema := func(name string) *sdcpb.SchemaElem {
		return &sdcpb.SchemaElem{
			Schema: &sdcpb.SchemaElem_Container{
				Container: &sdcpb.ContainerSchema{
					Name:       name,
					ModuleName: name,
					Fields: []*sdcpb.LeafSchema{
						{Name: "enabled", ModuleName: name, Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
					},
				},
			},
		}
	}
	fieldSchema := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{
			Field: &sdcpb.LeafSchema{
				Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"},
			},
		},
	}

	converter := NewConverter(&testSchemaClientBound{
		getSchemaPathFn: func(_ context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			elems := path.GetElem()
			if len(elems) == 0 {
				return nil, fmt.Errorf("empty path")
			}
			if elems[len(elems)-1].GetName() == "enabled" {
				return &sdcpb.GetSchemaResponse{Schema: fieldSchema}, nil
			}
			return &sdcpb.GetSchemaResponse{Schema: makeContainerSchema(elems[len(elems)-1].GetName())}, nil
		},
	})

	names := []string{
		"sonic-srv6",
		"sonic-bgp-global",
		"sonic-bgp-neighbor",
		"sonic-bgp-peergroup",
		"sonic-port",
		"sonic-interface",
		"sonic-loopback-interface",
	}
	batch := make([]*sdcpb.Update, 0, len(names))
	for _, name := range names {
		batch = append(batch, &sdcpb.Update{
			Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: name}}},
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_JsonIetfVal{
					JsonIetfVal: []byte(fmt.Sprintf(`{"%s:%s":{"enabled":"true"}}`, name, name)),
				},
			},
		})
	}

	updates, err := converter.ExpandUpdates(context.Background(), batch)
	if err != nil {
		t.Fatalf("ExpandUpdates returned error: %v", err)
	}
	if len(updates) != len(names) {
		t.Fatalf("expected %d updates, got %d", len(names), len(updates))
	}

	for i, upd := range updates {
		wantElems := []string{names[i], "enabled"}
		gotElems := upd.GetPath().GetElem()
		if len(gotElems) != len(wantElems) || gotElems[0].GetName() != wantElems[0] || gotElems[1].GetName() != wantElems[1] {
			t.Fatalf("update %d: expected path elems %v, got %v", i, wantElems, gotElems)
		}
		if got := upd.GetValue().GetStringVal(); got != "true" {
			t.Fatalf("update %d: expected value %q, got %q", i, "true", got)
		}
	}
}
