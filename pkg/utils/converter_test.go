package utils

import (
	"context"
	"encoding/json"
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
