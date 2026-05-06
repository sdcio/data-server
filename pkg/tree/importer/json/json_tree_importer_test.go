package json

import (
	"context"
	"reflect"
	"testing"

	"github.com/sdcio/data-server/pkg/tree/importer"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestJsonTreeImporter_NewConstructor(t *testing.T) {
	tests := []struct {
		name         string
		intentName   string
		priority     int32
		nonRevertive bool
		data         map[string]any
	}{
		{
			name:         "simple_intent",
			intentName:   "test-intent",
			priority:     int32(10),
			nonRevertive: false,
			data:         map[string]any{"foo": "bar"},
		},
		{
			name:         "non_revertive",
			intentName:   "non-revert-intent",
			priority:     int32(5),
			nonRevertive: true,
			data:         map[string]any{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imp := NewJsonTreeImporter(tt.data, tt.intentName, tt.priority, tt.nonRevertive)
			if imp == nil {
				t.Fatal("NewJsonTreeImporter() returned nil")
			}
			if got := imp.GetName(); got != tt.intentName {
				t.Errorf("GetName() = %q, want %q", got, tt.intentName)
			}
			if got := imp.GetPriority(); got != tt.priority {
				t.Errorf("GetPriority() = %d, want %d", got, tt.priority)
			}
			if got := imp.GetNonRevertive(); got != tt.nonRevertive {
				t.Errorf("GetNonRevertive() = %v, want %v", got, tt.nonRevertive)
			}
		})
	}
}

func TestJsonTreeImporter_ImplementsInterface(t *testing.T) {
	imp := NewJsonTreeImporter(map[string]any{}, "test", 1, false)
	if _, ok := interface{}(imp).(importer.ImportConfigAdapter); !ok {
		t.Error("NewJsonTreeImporter does not implement ImportConfigAdapter")
	}
}

func TestJsonTreeImporter_GetDeletes(t *testing.T) {
	imp := NewJsonTreeImporter(map[string]any{}, "test", 1, false)
	if imp.GetDeletes() == nil {
		t.Error("GetDeletes() returned nil, want empty PathSet")
	}
}

// TestJsonTreeImporter_GetName verifies element name is returned correctly.
func TestJsonTreeImporter_GetName(t *testing.T) {
	elem := newJsonTreeImporterElement("foo", nil)
	if got := elem.GetName(); got != "foo" {
		t.Errorf("GetName() = %q, want %q", got, "foo")
	}
}

// TestJsonTreeImporter_GetElement covers exact-key lookup (plain JSON, no prefix).
func TestJsonTreeImporter_GetElement(t *testing.T) {
	imp := NewJsonTreeImporter(map[string]any{"foo": "bar"}, "owner1", 5, false)
	got := imp.GetElement("foo")
	want := &JsonTreeImporterElement{data: "bar", name: "foo"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetElement() = %v, want %v", got, want)
	}
	if imp.GetElement("nonexistent") != nil {
		t.Error("GetElement(nonexistent) should return nil")
	}
}

// TestJsonTreeImporter_GetElement_LocalNameFallback verifies that GetElement finds a child
// by local name when the data key carries an RFC 7951 module prefix (e.g. "oc-if:name").
func TestJsonTreeImporter_GetElement_LocalNameFallback(t *testing.T) {
	data := map[string]any{
		"openconfig-interfaces:name": "eth0",
		"openconfig-interfaces:mtu":  9000,
	}
	elem := newJsonTreeImporterElement("interface", data)

	child := elem.GetElement("name")
	if child == nil {
		t.Fatal("GetElement(\"name\") returned nil; expected local-name fallback for \"openconfig-interfaces:name\"")
	}
	val, err := child.GetKeyValue(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetKeyValue() error: %v", err)
	}
	if val != "eth0" {
		t.Errorf("GetKeyValue() = %q, want %q", val, "eth0")
	}

	// Exact prefixed lookup must still work.
	if elem.GetElement("openconfig-interfaces:name") == nil {
		t.Error("GetElement(\"openconfig-interfaces:name\") returned nil; exact match should still work")
	}
}

// TestJsonTreeImporter_GetElements_NoPrefix verifies plain JSON keys pass through unchanged.
func TestJsonTreeImporter_GetElements_NoPrefix(t *testing.T) {
	imp := NewJsonTreeImporter(map[string]any{}, "test", 1, false)
	if n := len(imp.GetElements()); n != 0 {
		t.Errorf("GetElements() on empty data = %d, want 0", n)
	}

	imp2 := NewJsonTreeImporter(map[string]any{"interface": []any{map[string]any{"name": "eth0"}}}, "test", 1, false)
	elems := imp2.GetElements()
	if len(elems) != 1 {
		t.Fatalf("GetElements() = %d elements, want 1", len(elems))
	}
	if got := elems[0].GetName(); got != "interface" {
		t.Errorf("element name = %q, want %q", got, "interface")
	}
}

// TestJsonTreeImporter_GetElements_StripsPrefix verifies that RFC 7951 module prefixes
// (e.g. "openconfig-interfaces:interfaces") are stripped to the local name so the processor
// can look the element up in the schema tree by bare name.
func TestJsonTreeImporter_GetElements_StripsPrefix(t *testing.T) {
	data := map[string]any{
		"openconfig-interfaces:interfaces": map[string]any{"description": "top"},
	}
	imp := NewJsonTreeImporter(data, "test", 1, false)
	elems := imp.GetElements()
	if len(elems) != 1 {
		t.Fatalf("GetElements() = %d elements, want 1", len(elems))
	}
	if got := elems[0].GetName(); got != "interfaces" {
		t.Errorf("element name = %q, want %q", got, "interfaces")
	}
}

// TestJsonTreeImporter_ThreeLevelTraversal verifies traversal through a 3-level nested
// structure with RFC 7951 prefixed keys — prefixes stripped at each level.
func TestJsonTreeImporter_ThreeLevelTraversal(t *testing.T) {
	data := map[string]any{
		"openconfig-interfaces:interfaces": map[string]any{
			"openconfig-interfaces:interface": []any{
				map[string]any{"openconfig-interfaces:name": "eth0"},
			},
		},
	}
	imp := NewJsonTreeImporter(data, "test", 1, false)

	top := imp.GetElement("interfaces")
	if top == nil {
		t.Fatal("level-1 GetElement(\"interfaces\") returned nil")
	}
	mid := top.GetElements()
	if len(mid) != 1 {
		t.Fatalf("level-2 GetElements() = %d, want 1", len(mid))
	}
	if got := mid[0].GetName(); got != "interface" {
		t.Errorf("level-2 name = %q, want %q", got, "interface")
	}
	leaves := mid[0].GetElements()
	if len(leaves) != 1 {
		t.Fatalf("level-3 GetElements() = %d, want 1", len(leaves))
	}
	if got := leaves[0].GetName(); got != "name" {
		t.Errorf("level-3 name = %q, want %q", got, "name")
	}
}

func TestJsonTreeImporter_GetKeyValue(t *testing.T) {
	tests := []struct {
		name    string
		data    any
		wantVal string
	}{
		{"string", "bar", "bar"},
		{"int", 5, "5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elem := newJsonTreeImporterElement("x", tt.data)
			got, err := elem.GetKeyValue(context.Background(), nil)
			if err != nil {
				t.Fatalf("GetKeyValue() error: %v", err)
			}
			if got != tt.wantVal {
				t.Errorf("GetKeyValue() = %q, want %q", got, tt.wantVal)
			}
		})
	}
}

func TestJsonTreeImporter_GetTVValue(t *testing.T) {
	ctx := context.TODO()
	tests := []struct {
		name    string
		data    any
		slt     *sdcpb.SchemaLeafType
		want    *sdcpb.TypedValue
		wantErr bool
	}{
		{
			name: "string",
			data: "foobar",
			slt:  &sdcpb.SchemaLeafType{Type: "string"},
			want: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "foobar"}},
		},
		{
			name: "int32",
			data: int32(5),
			slt:  &sdcpb.SchemaLeafType{Type: "int32"},
			want: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 5}},
		},
		{
			name: "bool",
			data: true,
			slt:  &sdcpb.SchemaLeafType{Type: "boolean"},
			want: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: true}},
		},
		{
			name: "identityref_with_module_prefix",
			data: "openconfig-if-types:ETHERNET",
			slt: &sdcpb.SchemaLeafType{
				Type:                "identityref",
				IdentityPrefixesMap: map[string]string{"ETHERNET": "oc-if-types"},
				ModulePrefixMap:     map[string]string{"ETHERNET": "openconfig-if-types"},
			},
			want: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_IdentityrefVal{IdentityrefVal: &sdcpb.IdentityRef{
				Value:  "ETHERNET",
				Prefix: "oc-if-types",
				Module: "openconfig-if-types",
			}}},
		},
		{
			name:    "unknown_identityref_returns_error",
			data:    "some-module:UNKNOWN_IDENTITY",
			slt:     &sdcpb.SchemaLeafType{Type: "identityref", IdentityPrefixesMap: map[string]string{}, ModulePrefixMap: map[string]string{}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elem := &JsonTreeImporterElement{data: tt.data, name: "leaf"}
			got, err := elem.GetTVValue(ctx, tt.slt)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTVValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetTVValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestJsonTreeImporter_MultiKeyList verifies GetElement resolves multiple list keys where
// the JSON payload has module-prefixed key fields via the local-name fallback.
func TestJsonTreeImporter_MultiKeyList(t *testing.T) {
	data := map[string]any{
		"bgp:neighbor-address": "192.0.2.1",
		"bgp:local-as":         float64(65001),
		"bgp:peer-as":          float64(65002),
	}
	elem := newJsonTreeImporterElement("neighbor", data)

	localAs := elem.GetElement("local-as")
	if localAs == nil {
		t.Fatal("GetElement(\"local-as\") returned nil")
	}
	if v, _ := localAs.GetKeyValue(context.Background(), nil); v != "65001" {
		t.Errorf("local-as = %q, want %q", v, "65001")
	}

	neighborAddr := elem.GetElement("neighbor-address")
	if neighborAddr == nil {
		t.Fatal("GetElement(\"neighbor-address\") returned nil")
	}
	if v, _ := neighborAddr.GetKeyValue(context.Background(), nil); v != "192.0.2.1" {
		t.Errorf("neighbor-address = %q, want %q", v, "192.0.2.1")
	}
}

// TestJsonTreeImporter_LeafListRepeatedValues verifies GetElements returns all entries for a
// YANG leaf-list (including duplicates) as separate elements with the same local name.
func TestJsonTreeImporter_LeafListRepeatedValues(t *testing.T) {
	data := map[string]any{
		"oc-if:allowed-mtu": []any{float64(1500), float64(9000), float64(1500)},
	}
	elem := newJsonTreeImporterElement("interface", data)
	children := elem.GetElements()
	if len(children) != 3 {
		t.Fatalf("GetElements() = %d, want 3", len(children))
	}
	for i, child := range children {
		if got := child.GetName(); got != "allowed-mtu" {
			t.Errorf("element[%d].GetName() = %q, want %q", i, got, "allowed-mtu")
		}
	}
	for i, want := range []string{"1500", "9000", "1500"} {
		v, _ := children[i].GetKeyValue(context.Background(), nil)
		if v != want {
			t.Errorf("element[%d].GetKeyValue() = %q, want %q", i, v, want)
		}
	}
}

// TestJsonTreeImporter_MissingListKey verifies GetElement returns nil for an absent key.
func TestJsonTreeImporter_MissingListKey(t *testing.T) {
	elem := newJsonTreeImporterElement("interface", map[string]any{"oc-if:name": "eth0"})
	if elem.GetElement("description") != nil {
		t.Error("GetElement(\"description\") should return nil for absent key")
	}
}

// TestJsonTreeImporter_UnknownContainer verifies GetElement returns nil when the field name
// has no match (neither exact nor local-name).
func TestJsonTreeImporter_UnknownContainer(t *testing.T) {
	elem := newJsonTreeImporterElement("interface", map[string]any{"oc-if:name": "eth0", "oc-if:mtu": float64(1500)})
	if elem.GetElement("non-existent-container") != nil {
		t.Error("GetElement(\"non-existent-container\") should return nil")
	}
}
