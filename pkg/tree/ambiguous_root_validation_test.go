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
	ssSchema "github.com/sdcio/schema-server/pkg/schema"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNewEntry_rejectsBareAmbiguousRootChildViaSchemaServer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ambErr := &ssSchema.AmbiguousPathError{PathPrefix: "router", Modules: []string{"mod-a", "mod-b"}}
	mockSc := mockschema.NewMockClient(ctrl)
	mockSc.EXPECT().GetSchema(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req *sdcpb.GetSchemaRequest, _ ...interface{}) (*sdcpb.GetSchemaResponse, error) {
			path := req.GetPath()
			if path != nil && len(path.GetElem()) == 1 && path.GetElem()[0].GetName() == "router" && path.GetOrigin() == "" {
				return nil, status.Error(codes.FailedPrecondition, ambErr.Error())
			}
			return &sdcpb.GetSchemaResponse{
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Container{
						Container: &sdcpb.ContainerSchema{ModuleName: "mod-a"},
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

	_, err = api.NewEntry(ctx, root.Entry, api.LocalIdentity("router"), tc)
	if err == nil {
		t.Fatal("expected schema-server ambiguous path error for bare router")
	}

	_, err = api.NewEntry(ctx, root.Entry, api.NodeIdentity{Local: "router", Module: "mod-a"}, tc)
	if err != nil {
		t.Fatalf("module-qualified router should succeed: %v", err)
	}
}
