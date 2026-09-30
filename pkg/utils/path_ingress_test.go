package utils

import (
	"testing"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestStripGNMIPathPrefixes_PreservesRootModuleAsOrigin(t *testing.T) {
	p := &sdcpb.Path{
		IsRootBased: true,
		Elem:        []*sdcpb.PathElem{{Name: "Cisco-IOS-XR-um-router-static-cfg:router"}},
	}
	got := StripGNMIPathPrefixes(p)
	if got.GetOrigin() != "Cisco-IOS-XR-um-router-static-cfg" {
		t.Fatalf("Origin = %q, want Cisco-IOS-XR-um-router-static-cfg", got.GetOrigin())
	}
	if len(got.GetElem()) != 1 || got.GetElem()[0].GetName() != "router" {
		t.Fatalf("Elem = %v, want [router]", got.GetElem())
	}
}

func TestAnchorRootModuleFromJSONIETF_ModuleQualifiedWrapper(t *testing.T) {
	path := &sdcpb.Path{
		IsRootBased: true,
		Elem:        []*sdcpb.PathElem{{Name: "router"}},
	}
	jv := map[string]any{
		"Cisco-IOS-XR-um-router-ospf-cfg:router": map[string]any{"ospf": map[string]any{}},
	}
	if err := anchorRootModuleFromJSONIETF(path, jv); err != nil {
		t.Fatal(err)
	}
	if path.GetOrigin() != "Cisco-IOS-XR-um-router-ospf-cfg" {
		t.Fatalf("Origin = %q", path.GetOrigin())
	}
}

func TestAnchorRootModuleFromJSONIETF_EmptyPathSingleRootKey(t *testing.T) {
	path := &sdcpb.Path{}
	jv := map[string]any{
		"Cisco-IOS-XR-um-router-static-cfg:router": map[string]any{"static": map[string]any{}},
	}
	if err := anchorRootModuleFromJSONIETF(path, jv); err != nil {
		t.Fatal(err)
	}
	if path.GetOrigin() != "Cisco-IOS-XR-um-router-static-cfg" {
		t.Fatalf("Origin = %q", path.GetOrigin())
	}
	if len(path.GetElem()) != 1 || path.GetElem()[0].GetName() != "router" {
		t.Fatalf("Elem = %v", path.GetElem())
	}
}

func TestPathForSchemaLookup(t *testing.T) {
	p := &sdcpb.Path{
		Elem: []*sdcpb.PathElem{{Name: "mod-a:router"}},
	}
	got := PathForSchemaLookup(p)
	if got.GetOrigin() != "mod-a" || got.GetElem()[0].GetName() != "router" {
		t.Fatalf("lookup path = origin %q elem %q", got.GetOrigin(), got.GetElem()[0].GetName())
	}
}
