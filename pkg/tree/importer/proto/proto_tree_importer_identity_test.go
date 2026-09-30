package proto

import (
	"testing"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/sdc-protos/tree_persist"
)

func TestProtoTreeImporterElement_Identity_moduleQualifiedName(t *testing.T) {
	elem := &ProtoTreeImporterElement{
		data: &tree_persist.TreeElement{Name: "mod-a:router"},
	}
	want := api.NodeIdentity{Local: "router", Module: "mod-a"}
	if elem.Identity() != want {
		t.Fatalf("Identity() = %v want %v", elem.Identity(), want)
	}
}
