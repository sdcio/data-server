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
