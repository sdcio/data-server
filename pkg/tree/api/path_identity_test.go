package api_test

import (
	"testing"

	"github.com/sdcio/data-server/pkg/tree/api"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestNodeIdentityFromPathElem(t *testing.T) {
	path := &sdcpb.Path{
		Origin: "mod-a",
		Elem:   []*sdcpb.PathElem{sdcpb.NewPathElem("router", nil)},
	}
	got := api.NodeIdentityFromPathElem(path.Elem[0], path, 0)
	want := api.NodeIdentity{Local: "router", Module: "mod-a"}
	if got != want {
		t.Fatalf("first elem with origin: got %v want %v", got, want)
	}

	qualified := sdcpb.NewPathElem("mod-b:router", nil)
	got = api.NodeIdentityFromPathElem(qualified, path, 0)
	want = api.NodeIdentity{Local: "router", Module: "mod-b"}
	if got != want {
		t.Fatalf("prefixed name wins over origin: got %v want %v", got, want)
	}

	// Origin applies only to the first path element index.
	got = api.NodeIdentityFromPathElem(sdcpb.NewPathElem("child", nil), path, 1)
	if got.Module != "" {
		t.Fatalf("non-first index must not inherit origin, got %v", got)
	}
}

func TestLookupChild_collidingLocalNames(t *testing.T) {
	modA := api.NodeIdentity{Local: "router", Module: "mod-a"}
	modB := api.NodeIdentity{Local: "router", Module: "mod-b"}
	childs := map[string]api.Entry{
		modA.MapKey(): nil,
		modB.MapKey(): nil,
	}

	path := &sdcpb.Path{Origin: "mod-a", Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("router", nil)}}
	_, ok := api.LookupChild(childs, path.Elem[0], path, 0)
	if !ok {
		t.Fatal("expected mod-a router via origin")
	}

	qualified := &sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("mod-b:router", nil)}}
	_, ok = api.LookupChild(childs, qualified.Elem[0], qualified, 0)
	if !ok {
		t.Fatal("expected mod-b router via qualified name")
	}
}

func TestNodeIdentity_PersistName(t *testing.T) {
	if api.LocalIdentity("iface").PersistName() != "iface" {
		t.Fatal("local persist name")
	}
	id := api.NodeIdentity{Local: "router", Module: "mod-a"}
	if id.PersistName() != "mod-a:router" {
		t.Fatalf("PersistName() = %q", id.PersistName())
	}
}

func TestNodeIdentity_MatchesPathElemPrefix(t *testing.T) {
	id := api.NodeIdentity{Local: "router", Module: "mod-a"}
	if !id.MatchesPathElemPrefix(sdcpb.NewPathElem("mod-a:rou", nil)) {
		t.Fatal("expected prefix match on qualified partial")
	}
	if id.MatchesPathElemPrefix(sdcpb.NewPathElem("mod-b:router", nil)) {
		t.Fatal("mod-b prefix must not match mod-a identity")
	}
	if !api.LocalIdentity("interface").MatchesPathElemPrefix(sdcpb.NewPathElem("int", nil)) {
		t.Fatal("expected local prefix match")
	}
}

func TestApplyModuleToSchemaLookupPath(t *testing.T) {
	id := api.NodeIdentity{Local: "router", Module: "mod-a"}

	rootChild := &sdcpb.Path{
		Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("router", nil)},
	}
	api.ApplyModuleToSchemaLookupPath(rootChild, id, true)
	if rootChild.Origin != "mod-a" {
		t.Fatalf("root child lookup: origin %q want mod-a", rootChild.Origin)
	}

	nested := &sdcpb.Path{
		Elem: []*sdcpb.PathElem{
			sdcpb.NewPathElem("parent", nil),
			sdcpb.NewPathElem("router", nil),
		},
	}
	api.ApplyModuleToSchemaLookupPath(nested, id, false)
	last := nested.Elem[len(nested.Elem)-1]
	if last.Name != "mod-a:router" {
		t.Fatalf("nested lookup: last name %q want mod-a:router", last.Name)
	}
}
