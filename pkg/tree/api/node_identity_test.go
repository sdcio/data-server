package api_test

import (
	"testing"

	"github.com/sdcio/data-server/pkg/tree/api"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestNodeIdentity_MapKey(t *testing.T) {
	tests := []struct {
		name string
		id   api.NodeIdentity
		want string
	}{
		{"bare local", api.LocalIdentity("router"), "router"},
		{"module qualified", api.NodeIdentity{Local: "router", Module: "Cisco-IOS-XR-um-router-isis-cfg"}, "Cisco-IOS-XR-um-router-isis-cfg:router"},
		{"empty", api.NodeIdentity{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.MapKey(); got != tt.want {
				t.Errorf("MapKey() = %q, want %q", got, tt.want)
			}
			if got := tt.id.JSONIETFKey(); got != tt.want {
				t.Errorf("JSONIETFKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNodeIdentity_GNMIOrigin(t *testing.T) {
	id := api.NodeIdentity{Local: "router", Module: "mod-a"}
	if got := id.GNMIOrigin(); got != "mod-a" {
		t.Errorf("GNMIOrigin() = %q, want mod-a", got)
	}
	if got := api.LocalIdentity("x").GNMIOrigin(); got != "" {
		t.Errorf("GNMIOrigin() for local-only = %q, want empty", got)
	}
}

func TestParseJSONIETFKey(t *testing.T) {
	got := api.ParseJSONIETFKey("mod-a:router")
	want := api.NodeIdentity{Local: "router", Module: "mod-a"}
	if got != want {
		t.Fatalf("ParseJSONIETFKey = %+v, want %+v", got, want)
	}
	if api.ParseJSONIETFKey("interface") != api.LocalIdentity("interface") {
		t.Fatal("expected bare key to parse as local-only identity")
	}
}

func TestValidateIdentityMatchesSchema(t *testing.T) {
	schema := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Container{
			Container: &sdcpb.ContainerSchema{ModuleName: "mod-a"},
		},
	}
	if err := api.ValidateIdentityMatchesSchema(api.NodeIdentity{Local: "x", Module: "mod-a"}, schema); err != nil {
		t.Fatalf("matching module: %v", err)
	}
	if err := api.ValidateIdentityMatchesSchema(api.LocalIdentity("x"), schema); err != nil {
		t.Fatalf("empty identity module: %v", err)
	}
	if err := api.ValidateIdentityMatchesSchema(api.NodeIdentity{Local: "x", Module: "mod-b"}, schema); err == nil {
		t.Fatal("expected mismatch error")
	}
}
