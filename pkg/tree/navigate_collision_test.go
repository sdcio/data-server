package tree

import (
	"context"
	"runtime"
	"testing"

	"github.com/sdcio/data-server/mocks/mockschema"
	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/ops"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

func newCollisionTestRoot(t *testing.T) (context.Context, api.Entry) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockSc := mockschema.NewMockClient(ctrl)
	mockSc.EXPECT().GetSchema(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req *sdcpb.GetSchemaRequest, _ ...interface{}) (*sdcpb.GetSchemaResponse, error) {
			mod := req.GetPath().GetOrigin()
			if mod == "" {
				mod = "mod-a"
			}
			return &sdcpb.GetSchemaResponse{
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Container{
						Container: &sdcpb.ContainerSchema{ModuleName: mod},
					},
				},
			}, nil
		},
	).AnyTimes()

	schemaCfg := &config.SchemaConfig{Name: "test", Vendor: "v", Version: "1"}
	scb := schemaClient.NewSchemaClientBound(schemaCfg, mockSc)
	ctx := context.Background()
	tc := NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatalf("NewTreeRoot: %v", err)
	}

	for _, mod := range []string{"mod-a", "mod-b"} {
		_, err = api.NewEntry(ctx, root.Entry, api.NodeIdentity{Local: "router", Module: mod}, tc)
		if err != nil {
			t.Fatalf("NewEntry %s: %v", mod, err)
		}
	}
	return ctx, root.Entry
}

func TestNavigateSdcpbPath_collidingRootChildren(t *testing.T) {
	ctx, root := newCollisionTestRoot(t)

	pathA := &sdcpb.Path{
		Origin: "mod-a",
		Elem:   []*sdcpb.PathElem{sdcpb.NewPathElem("router", nil)},
	}
	entryA, err := ops.NavigateSdcpbPath(ctx, root, pathA)
	if err != nil {
		t.Fatalf("navigate mod-a: %v", err)
	}
	if entryA.Identity().Module != "mod-a" {
		t.Fatalf("got module %q want mod-a", entryA.Identity().Module)
	}

	pathB := &sdcpb.Path{
		Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("mod-b:router", nil)},
	}
	entryB, err := ops.NavigateSdcpbPath(ctx, root, pathB)
	if err != nil {
		t.Fatalf("navigate mod-b: %v", err)
	}
	if entryB.Identity().Module != "mod-b" {
		t.Fatalf("got module %q want mod-b", entryB.Identity().Module)
	}
	if entryA == entryB {
		t.Fatal("expected distinct tree nodes for mod-a and mod-b router")
	}
}
