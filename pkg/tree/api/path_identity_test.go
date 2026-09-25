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
