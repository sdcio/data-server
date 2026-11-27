package utils

import (
	"context"
	"encoding/json"
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

func newMapSchemaConverter(schemaMap map[string]*sdcpb.GetSchemaResponse) *Converter {
	return NewConverter(&testSchemaClientBound{
		getSchemaPathFn: func(_ context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			key := ""
			for i, pe := range path.GetElem() {
				if i > 0 {
					key += "/"
				}
				key += pe.GetName()
			}
			if rsp, ok := schemaMap[key]; ok {
				return rsp, nil
			}
			return &sdcpb.GetSchemaResponse{Schema: &sdcpb.SchemaElem{}}, nil
		},
	})
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

// TestExpandContainerValueSkipsStateLeaves verifies that ExpandUpdate (and thus
// ExpandContainerValue) does not emit leaf updates for YANG "config false"
// leaves, even when they arrive inside a JSON-IETF blob at the container level.
//
// This is the regression test for the SRL delete failure observed in CI: the
// BGP AFI-SAFI container was being delivered by the gNMI subscription as a
// JSON blob that contained both configurable leaves (e.g. admin-state) and
// read-only / operational leaves (e.g. active-routes).  Before the fix, all
// leaves were written into the syncTree under the running owner.  When an
// intent that owned the same container was later deleted, the mandatory-field
// validator would fire on the container (kept alive by the running-owner state
// leaf) and fail with "mandatory child does not exist", blocking the delete
// for the full retry window.
func TestExpandContainerValueSkipsStateLeaves(t *testing.T) {
	// Schema map:
	//   /afi-safi                     → container (not state)
	//   /afi-safi/admin-state         → field, IsState=false  (config leaf)
	//   /afi-safi/active-routes       → field, IsState=true   (state leaf)
	//   /afi-safi/prefixes            → container, IsState=true (state sub-tree)
	//   /afi-safi/prefixes/installed  → field, IsState=true
	schemaMap := map[string]*sdcpb.GetSchemaResponse{
		"afi-safi": {
			Schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Container{
					Container: &sdcpb.ContainerSchema{
						Name:     "afi-safi",
						IsState:  false,
						Fields:   []*sdcpb.LeafSchema{{Name: "admin-state"}, {Name: "active-routes"}},
						Children: []string{"prefixes"},
					},
				},
			},
		},
		"afi-safi/admin-state": {
			Schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Field{
					Field: &sdcpb.LeafSchema{
						Name:    "admin-state",
						IsState: false,
						Type:    &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"},
					},
				},
			},
		},
		"afi-safi/active-routes": {
			Schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Field{
					Field: &sdcpb.LeafSchema{
						Name:    "active-routes",
						IsState: true,
						Type:    &sdcpb.SchemaLeafType{Type: "uint32", TypeName: "uint32"},
					},
				},
			},
		},
		"afi-safi/prefixes": {
			Schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Container{
					Container: &sdcpb.ContainerSchema{
						Name:     "prefixes",
						IsState:  true,
						Fields:   []*sdcpb.LeafSchema{{Name: "installed"}},
					},
				},
			},
		},
		"afi-safi/prefixes/installed": {
			Schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Field{
					Field: &sdcpb.LeafSchema{
						Name:    "installed",
						IsState: true,
						Type:    &sdcpb.SchemaLeafType{Type: "uint32", TypeName: "uint32"},
					},
				},
			},
		},
	}

	scb := &testSchemaClientBound{
		getSchemaPathFn: func(_ context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			key := ""
			for i, pe := range path.GetElem() {
				if i > 0 {
					key += "/"
				}
				key += pe.GetName()
			}
			if rsp, ok := schemaMap[key]; ok {
				return rsp, nil
			}
			// Return an empty response for unknown paths (will cause getItem to skip)
			return &sdcpb.GetSchemaResponse{Schema: &sdcpb.SchemaElem{}}, nil
		},
	}

	body, err := json.Marshal(map[string]any{
		"admin-state":   "enable",
		"active-routes": 42,
		"prefixes":      map[string]any{"installed": 10},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	converter := NewConverter(scb)
	updates, err := converter.ExpandUpdate(context.Background(), &sdcpb.Update{
		Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "afi-safi"}}},
		Value: &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: body},
		},
	})
	if err != nil {
		t.Fatalf("ExpandUpdate returned error: %v", err)
	}

	// Only the config leaf (admin-state) should be emitted; the two state
	// leaves (active-routes, prefixes/installed) must be absent.
	if len(updates) != 1 {
		names := make([]string, 0, len(updates))
		for _, u := range updates {
			elems := u.GetPath().GetElem()
			names = append(names, elems[len(elems)-1].GetName())
		}
		t.Fatalf("expected 1 update (admin-state only), got %d: %v", len(updates), names)
	}
	leaf := updates[0].GetPath().GetElem()
	if leaf[len(leaf)-1].GetName() != "admin-state" {
		t.Fatalf("expected leaf admin-state, got %q", leaf[len(leaf)-1].GetName())
	}
}

// TestExpandContainerValueStripsIdentityrefModulePrefixFromPathKey verifies
// that an identityref list key arriving via the gNMI *path* (rather than
// nested in the JSON-IETF value) has its module prefix stripped exactly like
// the JSON-value branch already does.
//
// This is the regression test for the SRL delete failure observed in CI:
// device sync (GET) delivered the "afi-safi" list key already embedded in
// the path as "srl_nokia-common:ipv4-unicast", while intents key the same
// list entry as unprefixed "ipv4-unicast". Without stripping the prefix on
// the path-key branch too, these resolved to two different tree nodes for
// the same list entry: an intent-owned one (fully deleted with the intent)
// and a running-only "ghost" one that was never cleaned up. The ghost node
// kept the ancestor "bgp" container's mandatory-field validator alive after
// the owning intent was deleted, blocking the delete transaction for the
// full 15-minute CI retry window.
func TestExpandContainerValueStripsIdentityrefModulePrefixFromPathKey(t *testing.T) {
	schemaMap := map[string]*sdcpb.GetSchemaResponse{
		"afi-safi": {
			Schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Container{
					Container: &sdcpb.ContainerSchema{
						Name:   "afi-safi",
						Keys:   []*sdcpb.LeafSchema{{Name: "afi-safi-name", Type: &sdcpb.SchemaLeafType{Type: "identityref"}}},
						Fields: []*sdcpb.LeafSchema{{Name: "admin-state"}},
					},
				},
			},
		},
		"afi-safi/admin-state": {
			Schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Field{
					Field: &sdcpb.LeafSchema{
						Name: "admin-state",
						Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"},
					},
				},
			},
		},
	}

	scb := &testSchemaClientBound{
		getSchemaPathFn: func(_ context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			key := ""
			for i, pe := range path.GetElem() {
				if i > 0 {
					key += "/"
				}
				key += pe.GetName()
			}
			if rsp, ok := schemaMap[key]; ok {
				return rsp, nil
			}
			return &sdcpb.GetSchemaResponse{Schema: &sdcpb.SchemaElem{}}, nil
		},
	}

	body, err := json.Marshal(map[string]any{
		"admin-state": "enable",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	converter := NewConverter(scb)
	updates, err := converter.ExpandUpdate(context.Background(), &sdcpb.Update{
		Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
			{Name: "afi-safi", Key: map[string]string{"afi-safi-name": "srl_nokia-common:ipv4-unicast"}},
		}},
		Value: &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: body},
		},
	})
	if err != nil {
		t.Fatalf("ExpandUpdate returned error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}

	elems := updates[0].GetPath().GetElem()
	got := elems[len(elems)-2].GetKey()["afi-safi-name"]
	if got != "ipv4-unicast" {
		t.Fatalf("expected module prefix stripped from path key, got %q", got)
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

func TestExpandUpdateContainerJSONNullReturnsEmpty(t *testing.T) {
	converter := NewConverter(&testSchemaClientBound{
		getSchemaPathFn: func(context.Context, *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			return &sdcpb.GetSchemaResponse{
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Container{
						Container: &sdcpb.ContainerSchema{
							Name:   "LbRtdIf-list",
							Keys:   []*sdcpb.LeafSchema{{Name: "id", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}}},
							Fields: []*sdcpb.LeafSchema{{Name: "descr", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}}},
						},
					},
				},
			}, nil
		},
	})

	for _, tc := range []struct {
		name string
		val  *sdcpb.TypedValue
	}{
		{
			name: "json_val",
			val:  &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte("null")}},
		},
		{
			name: "json_ietf_val",
			val:  &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: []byte("null")}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			updates, err := converter.ExpandUpdate(context.Background(), &sdcpb.Update{
				Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
					{Name: "LbRtdIf-list", Key: map[string]string{"id": "lo99"}},
				}},
				Value: tc.val,
			})
			if err != nil {
				t.Fatalf("ExpandUpdate returned error: %v", err)
			}
			if len(updates) != 0 {
				t.Fatalf("expected 0 updates, got %d: %v", len(updates), updates)
			}
		})
	}
}

func TestExpandUpdateFieldAndNestedMembersJSONNullSkipped(t *testing.T) {
	t.Run("field_leaf_types", func(t *testing.T) {
		for _, leafType := range []string{"string", "uint32", "boolean", "decimal64", "enumeration", "empty"} {
			t.Run(leafType, func(t *testing.T) {
				converter := NewConverter(&testSchemaClientBound{
					getSchemaPathFn: func(context.Context, *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
						return &sdcpb.GetSchemaResponse{
							Schema: &sdcpb.SchemaElem{
								Schema: &sdcpb.SchemaElem_Field{
									Field: &sdcpb.LeafSchema{
										Name: "leaf",
										Type: &sdcpb.SchemaLeafType{Type: leafType, TypeName: leafType},
									},
								},
							},
						}, nil
					},
				})

				for _, tv := range []*sdcpb.TypedValue{
					{Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte("null")}},
					{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: []byte("null")}},
				} {
					updates, err := converter.ExpandUpdate(context.Background(), &sdcpb.Update{
						Path:  &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "root"}, {Name: "leaf"}}},
						Value: tv,
					})
					if err != nil {
						t.Fatalf("ExpandUpdate returned error for %s: %v", leafType, err)
					}
					if len(updates) != 0 {
						t.Fatalf("expected 0 updates for null %s, got %d: %v", leafType, len(updates), updates)
					}
				}
			})
		}
	})

	t.Run("field_empty_array_null_preserved", func(t *testing.T) {
		converter := NewConverter(&testSchemaClientBound{
			getSchemaPathFn: func(context.Context, *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
				return &sdcpb.GetSchemaResponse{
					Schema: &sdcpb.SchemaElem{
						Schema: &sdcpb.SchemaElem_Field{
							Field: &sdcpb.LeafSchema{
								Name: "empty-leaf",
								Type: &sdcpb.SchemaLeafType{Type: "empty", TypeName: "empty"},
							},
						},
					},
				}, nil
			},
		})

		updates, err := converter.ExpandUpdate(context.Background(), &sdcpb.Update{
			Path:  &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "root"}, {Name: "empty-leaf"}}},
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: []byte("[null]")}},
		})
		if err != nil {
			t.Fatalf("ExpandUpdate returned error: %v", err)
		}
		if len(updates) != 1 || updates[0].GetValue().GetEmptyVal() == nil {
			t.Fatalf("expected 1 EmptyVal update for [null], got %v", updates)
		}
	})

	t.Run("nested_null_members_in_container", func(t *testing.T) {
		schemaMap := map[string]*sdcpb.GetSchemaResponse{
			"iface": {
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Container{
						Container: &sdcpb.ContainerSchema{
							Name: "iface",
							Fields: []*sdcpb.LeafSchema{
								{Name: "admin-state", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
								{Name: "descr", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
								{Name: "flag", Type: &sdcpb.SchemaLeafType{Type: "empty", TypeName: "empty"}},
							},
							Leaflists: []*sdcpb.LeafListSchema{
								{Name: "tags", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
							},
							Children: []string{"sub", "present-sub"},
						},
					},
				},
			},
			"iface/admin-state": {
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Field{
						Field: &sdcpb.LeafSchema{Name: "admin-state", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
					},
				},
			},
			"iface/descr": {
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Field{
						Field: &sdcpb.LeafSchema{Name: "descr", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
					},
				},
			},
			"iface/flag": {
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Field{
						Field: &sdcpb.LeafSchema{Name: "flag", Type: &sdcpb.SchemaLeafType{Type: "empty", TypeName: "empty"}},
					},
				},
			},
			"iface/tags": {
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Leaflist{
						Leaflist: &sdcpb.LeafListSchema{Name: "tags", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
					},
				},
			},
			"iface/sub": {
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Container{
						Container: &sdcpb.ContainerSchema{
							Name:       "sub",
							IsPresence: true,
							Fields: []*sdcpb.LeafSchema{
								{Name: "descr", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
							},
						},
					},
				},
			},
			"iface/sub/descr": {
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Field{
						Field: &sdcpb.LeafSchema{Name: "descr", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
					},
				},
			},
			"iface/present-sub": {
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Container{
						Container: &sdcpb.ContainerSchema{
							Name:       "present-sub",
							IsPresence: true,
							Fields: []*sdcpb.LeafSchema{
								{Name: "descr", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
							},
						},
					},
				},
			},
			"iface/present-sub/descr": {
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Field{
						Field: &sdcpb.LeafSchema{Name: "descr", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
					},
				},
			},
		}

		converter := newMapSchemaConverter(schemaMap)

		updates, err := converter.ExpandUpdate(context.Background(), &sdcpb.Update{
			Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "iface"}}},
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_JsonVal{
					JsonVal: []byte(`{"admin-state":"up","descr":null,"flag":null,"tags":null,"sub":null,"present-sub":{"descr":null}}`),
				},
			},
		})
		if err != nil {
			t.Fatalf("ExpandUpdate returned error: %v", err)
		}
		if len(updates) != 2 {
			t.Fatalf("expected 2 updates (admin-state and present-sub EmptyVal), got %d: %v", len(updates), updates)
		}
		gotByLeaf := map[string]*sdcpb.TypedValue{}
		for _, u := range updates {
			elems := u.GetPath().GetElem()
			gotByLeaf[elems[len(elems)-1].GetName()] = u.GetValue()
		}
		if gotByLeaf["admin-state"].GetStringVal() != "up" {
			t.Fatalf("expected admin-state %q, got %v", "up", gotByLeaf["admin-state"])
		}
		if _, ok := gotByLeaf["present-sub"].GetValue().(*sdcpb.TypedValue_EmptyVal); !ok {
			t.Fatalf("expected EmptyVal for presence container with null child leaf, got %v", gotByLeaf["present-sub"])
		}
	})

	t.Run("null_leaflist_elements_skipped", func(t *testing.T) {
		tagsSchema := &sdcpb.LeafListSchema{Name: "tags", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}}
		converter := newMapSchemaConverter(map[string]*sdcpb.GetSchemaResponse{
			"iface": {
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Container{
						Container: &sdcpb.ContainerSchema{Name: "iface", Leaflists: []*sdcpb.LeafListSchema{tagsSchema}},
					},
				},
			},
			"iface/tags": {
				Schema: &sdcpb.SchemaElem{Schema: &sdcpb.SchemaElem_Leaflist{Leaflist: tagsSchema}},
			},
		})

		updates, err := converter.ExpandUpdate(context.Background(), &sdcpb.Update{
			Path:  &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "iface"}}},
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(`{"tags":["a",null,"b"]}`)}},
		})
		if err != nil {
			t.Fatalf("ExpandUpdate returned error: %v", err)
		}
		if len(updates) != 1 {
			t.Fatalf("expected 1 leaf-list update, got %d: %v", len(updates), updates)
		}
		var got []string
		for _, e := range updates[0].GetValue().GetLeaflistVal().GetElement() {
			got = append(got, e.GetStringVal())
		}
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Fatalf("expected leaf-list [a b], got %v", got)
		}
	})
}

func TestExpandUpdateLeaflistNullOrEmptyJSONReturnsEmpty(t *testing.T) {
	converter := NewConverter(&testSchemaClientBound{
		getSchemaPathFn: func(context.Context, *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			return &sdcpb.GetSchemaResponse{
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Leaflist{
						Leaflist: &sdcpb.LeafListSchema{Name: "tags", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
					},
				},
			}, nil
		},
	})

	for _, pl := range []struct {
		name string
		raw  []byte
	}{
		{name: "nil_bytes", raw: nil},
		{name: "empty_bytes", raw: []byte{}},
		{name: "whitespace_only", raw: []byte(" \t\r\n ")},
		{name: "null", raw: []byte("null")},
	} {
		for _, enc := range []struct {
			name string
			tv   *sdcpb.TypedValue
		}{
			{name: "json_val", tv: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: pl.raw}}},
			{name: "json_ietf_val", tv: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: pl.raw}}},
		} {
			t.Run(pl.name+"/"+enc.name, func(t *testing.T) {
				updates, err := converter.ExpandUpdate(context.Background(), &sdcpb.Update{
					Path:  &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "iface"}, {Name: "tags"}}},
					Value: enc.tv,
				})
				if err != nil {
					t.Fatalf("ExpandUpdate returned error: %v", err)
				}
				if len(updates) != 0 {
					t.Fatalf("expected 0 updates, got %d: %v", len(updates), updates)
				}
			})
		}
	}
}

func TestExpandUpdateEmptyOrWhitespaceJSONReturnsEmpty(t *testing.T) {
	containerSchema := &sdcpb.GetSchemaResponse{
		Schema: &sdcpb.SchemaElem{
			Schema: &sdcpb.SchemaElem_Container{
				Container: &sdcpb.ContainerSchema{
					Name:   "ntpq-items",
					Fields: []*sdcpb.LeafSchema{{Name: "adminSt", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}}},
				},
			},
		},
	}
	fieldSchema := &sdcpb.GetSchemaResponse{
		Schema: &sdcpb.SchemaElem{
			Schema: &sdcpb.SchemaElem_Field{
				Field: &sdcpb.LeafSchema{
					Name: "adminSt",
					Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"},
				},
			},
		},
	}

	converter := NewConverter(&testSchemaClientBound{
		getSchemaPathFn: func(_ context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			elems := path.GetElem()
			if len(elems) == 2 {
				return fieldSchema, nil
			}
			return containerSchema, nil
		},
	})

	payloads := []struct {
		name string
		raw  []byte
	}{
		{name: "nil_bytes", raw: nil},
		{name: "empty_bytes", raw: []byte{}},
		{name: "whitespace_only", raw: []byte(" \t\r\n ")},
	}

	paths := []struct {
		name string
		path *sdcpb.Path
	}{
		{name: "container", path: &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "ntpq-items"}}}},
		{name: "field", path: &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "ntpq-items"}, {Name: "adminSt"}}}},
	}

	for _, p := range paths {
		for _, pl := range payloads {
			for _, enc := range []struct {
				name string
				tv   *sdcpb.TypedValue
			}{
				{name: "json_val", tv: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: pl.raw}}},
				{name: "json_ietf_val", tv: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: pl.raw}}},
			} {
				t.Run(p.name+"/"+pl.name+"/"+enc.name, func(t *testing.T) {
					updates, err := converter.ExpandUpdate(context.Background(), &sdcpb.Update{
						Path:  p.path,
						Value: enc.tv,
					})
					if err != nil {
						t.Fatalf("ExpandUpdate returned error: %v", err)
					}
					if len(updates) != 0 {
						t.Fatalf("expected 0 updates, got %d: %v", len(updates), updates)
					}
				})
			}
		}
	}
}

func TestExpandContainerValueDuplicateKeyInPathAndJSON(t *testing.T) {
	lbSchemaMap := map[string]*sdcpb.GetSchemaResponse{
		"LbRtdIf-list": {
			Schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Container{
					Container: &sdcpb.ContainerSchema{
						Name:       "LbRtdIf-list",
						ModuleName: "Cisco-NX-OS-device",
						Keys: []*sdcpb.LeafSchema{
							{Name: "id", ModuleName: "Cisco-NX-OS-device", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
						},
						Fields: []*sdcpb.LeafSchema{
							{Name: "adminSt", ModuleName: "Cisco-NX-OS-device", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
						},
					},
				},
			},
		},
		"LbRtdIf-list/id": {
			Schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Field{
					Field: &sdcpb.LeafSchema{
						Name: "id",
						Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"},
					},
				},
			},
		},
		"LbRtdIf-list/adminSt": {
			Schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Field{
					Field: &sdcpb.LeafSchema{
						Name: "adminSt",
						Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"},
					},
				},
			},
		},
	}

	lbConverter := newMapSchemaConverter(lbSchemaMap)

	t.Run("matching_key_in_single_element_array", func(t *testing.T) {
		updates, err := lbConverter.ExpandUpdate(context.Background(), &sdcpb.Update{
			Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "LbRtdIf-list", Key: map[string]string{"id": "lo99"}},
			}},
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_JsonVal{
					JsonVal: []byte(`[{"id":"lo99","adminSt":"up"}]`),
				},
			},
		})
		if err != nil {
			t.Fatalf("ExpandUpdate returned error: %v", err)
		}
		if len(updates) != 2 {
			t.Fatalf("expected 2 updates (id and adminSt), got %d: %v", len(updates), updates)
		}
		got := map[string]string{}
		for _, u := range updates {
			elems := u.GetPath().GetElem()
			if elems[0].GetKey()["id"] != "lo99" {
				t.Fatalf("expected key id=lo99 on path, got %v", elems[0].GetKey())
			}
			got[elems[len(elems)-1].GetName()] = u.GetValue().GetStringVal()
		}
		if got["id"] != "lo99" || got["adminSt"] != "up" {
			t.Fatalf("unexpected leaf values: %v", got)
		}
	})

	t.Run("matching_key_in_object", func(t *testing.T) {
		updates, err := lbConverter.ExpandUpdate(context.Background(), &sdcpb.Update{
			Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "LbRtdIf-list", Key: map[string]string{"id": "lo99"}},
			}},
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_JsonVal{
					JsonVal: []byte(`{"id":"lo99","adminSt":"up"}`),
				},
			},
		})
		if err != nil {
			t.Fatalf("ExpandUpdate returned error: %v", err)
		}
		if len(updates) != 2 {
			t.Fatalf("expected 2 updates, got %d: %v", len(updates), updates)
		}
	})

	t.Run("conflicting_key_in_path_and_json_errors", func(t *testing.T) {
		_, err := lbConverter.ExpandUpdate(context.Background(), &sdcpb.Update{
			Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "LbRtdIf-list", Key: map[string]string{"id": "lo99"}},
			}},
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_JsonVal{
					JsonVal: []byte(`[{"id":"lo100","adminSt":"up"}]`),
				},
			},
		})
		if err == nil {
			t.Fatal("expected error for conflicting key between path and JSON value, got nil")
		}
	})

	t.Run("matching_identityref_key_with_module_prefixes", func(t *testing.T) {
		idRefType := &sdcpb.SchemaLeafType{
			Type:                "identityref",
			TypeName:            "afi-safi-type",
			IdentityPrefixesMap: map[string]string{"ipv4-unicast": "srl_nokia-common", "ipv6-unicast": "srl_nokia-common"},
			ModulePrefixMap:     map[string]string{"ipv4-unicast": "srl_nokia-common", "ipv6-unicast": "srl_nokia-common"},
		}
		afiSchemaMap := map[string]*sdcpb.GetSchemaResponse{
			"afi-safi": {
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Container{
						Container: &sdcpb.ContainerSchema{
							Name:       "afi-safi",
							ModuleName: "srl_nokia-bgp",
							Keys: []*sdcpb.LeafSchema{
								{
									Name:       "afi-safi-name",
									ModuleName: "srl_nokia-bgp",
									Type:       &sdcpb.SchemaLeafType{Type: "leafref", LeafrefTargetType: idRefType},
								},
							},
							Fields: []*sdcpb.LeafSchema{
								{Name: "admin-state", ModuleName: "srl_nokia-bgp", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}},
							},
						},
					},
				},
			},
			"afi-safi/afi-safi-name": {
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Field{
						Field: &sdcpb.LeafSchema{
							Name: "afi-safi-name",
							Type: &sdcpb.SchemaLeafType{Type: "leafref", LeafrefTargetType: idRefType},
						},
					},
				},
			},
			"afi-safi/admin-state": {
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Field{
						Field: &sdcpb.LeafSchema{
							Name: "admin-state",
							Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"},
						},
					},
				},
			},
		}

		afiConverter := newMapSchemaConverter(afiSchemaMap)

		updates, err := afiConverter.ExpandUpdate(context.Background(), &sdcpb.Update{
			Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "afi-safi", Key: map[string]string{"afi-safi-name": "srl_nokia-common:ipv4-unicast"}},
			}},
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_JsonIetfVal{
					JsonIetfVal: []byte(`{"srl_nokia-bgp:afi-safi-name":"ipv4-unicast","admin-state":"enable"}`),
				},
			},
		})
		if err != nil {
			t.Fatalf("ExpandUpdate returned error: %v", err)
		}
		if len(updates) != 2 {
			t.Fatalf("expected 2 updates, got %d: %v", len(updates), updates)
		}
		for _, u := range updates {
			if gotKey := u.GetPath().GetElem()[0].GetKey()["afi-safi-name"]; gotKey != "ipv4-unicast" {
				t.Fatalf("expected normalized key ipv4-unicast, got %q", gotKey)
			}
		}

		_, err = afiConverter.ExpandUpdate(context.Background(), &sdcpb.Update{
			Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "afi-safi", Key: map[string]string{"afi-safi-name": "srl_nokia-common:ipv4-unicast"}},
			}},
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_JsonIetfVal{
					JsonIetfVal: []byte(`{"afi-safi-name":"srl_nokia-common:ipv6-unicast","admin-state":"enable"}`),
				},
			},
		})
		if err == nil {
			t.Fatal("expected error for conflicting identityref key between path and JSON value, got nil")
		}

		_, err = afiConverter.ExpandUpdate(context.Background(), &sdcpb.Update{
			Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "afi-safi", Key: map[string]string{"afi-safi-name": "srl_nokia-common:ipv4-unicast"}},
			}},
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_JsonIetfVal{
					JsonIetfVal: []byte(`{"afi-safi-name":"other-mod:ipv4-unicast","admin-state":"enable"}`),
				},
			},
		})
		if err == nil {
			t.Fatal("expected error for conflicting explicit module prefixes on identityref key, got nil")
		}

		updates, err = afiConverter.ExpandUpdate(context.Background(), &sdcpb.Update{
			Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "afi-safi", Key: map[string]string{"afi-safi-name": "other-mod:l2vpn-evpn"}},
			}},
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"admin-state":"enable"}`)},
			},
		})
		if err != nil {
			t.Fatalf("expected qualifier of identity absent from prefix maps to be stripped, got error: %v", err)
		}
		for _, u := range updates {
			if gotKey := u.GetPath().GetElem()[0].GetKey()["afi-safi-name"]; gotKey != "l2vpn-evpn" {
				t.Fatalf("expected normalized key l2vpn-evpn, got %q", gotKey)
			}
		}
	})
}

func TestExpandUpdatesBatchPreservesSiblingsAroundNullAndEmpty(t *testing.T) {
	converter := newMapSchemaConverter(map[string]*sdcpb.GetSchemaResponse{
		"empty-cont": {
			Schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Container{
					Container: &sdcpb.ContainerSchema{Name: "empty-cont"},
				},
			},
		},
		"missing-entry": {
			Schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Container{
					Container: &sdcpb.ContainerSchema{
						Name: "missing-entry",
						Keys: []*sdcpb.LeafSchema{{Name: "id", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}}},
					},
				},
			},
		},
		"live-cont": {
			Schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Container{
					Container: &sdcpb.ContainerSchema{
						Name:   "live-cont",
						Fields: []*sdcpb.LeafSchema{{Name: "hostname", Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"}}},
					},
				},
			},
		},
		"live-cont/hostname": {
			Schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Field{
					Field: &sdcpb.LeafSchema{
						Name: "hostname",
						Type: &sdcpb.SchemaLeafType{Type: "string", TypeName: "string"},
					},
				},
			},
		},
	})

	updates, err := converter.ExpandUpdates(context.Background(), []*sdcpb.Update{
		{
			Path:  &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "empty-cont"}}},
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte("")}},
		},
		{
			Path:  &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "missing-entry", Key: map[string]string{"id": "lo99"}}}},
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte("null")}},
		},
		{
			Path:  &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "live-cont"}}},
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(`{"hostname":"n9k-leaf1"}`)}},
		},
	})
	if err != nil {
		t.Fatalf("ExpandUpdates returned error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update from live-cont, got %d: %v", len(updates), updates)
	}
	if got := updates[0].GetValue().GetStringVal(); got != "n9k-leaf1" {
		t.Fatalf("expected hostname %q, got %q", "n9k-leaf1", got)
	}
}
