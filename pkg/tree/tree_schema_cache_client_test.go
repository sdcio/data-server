package tree

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sdcio/data-server/mocks/mockschemaclientbound"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// TestTreeSchemaCacheClientImpl_GetSchema test that the TreeSchemaCacheClientImpl caches SchemaRequests
func TestTreeSchemaCacheClientImpl_GetSchema(t *testing.T) {

	ctx := context.TODO()

	x, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Error(err)
	}

	sdcpbSchema := &sdcpb.Schema{
		Name:    schema.Name,
		Vendor:  schema.Vendor,
		Version: schema.Version,
	}

	mockCtrl := gomock.NewController(t)
	mockscb := mockschemaclientbound.NewMockSchemaClientBound(mockCtrl)

	// make the mock respond to GetSchema requests
	mockscb.EXPECT().GetSchema(gomock.Any(), gomock.Any()).Times(3).DoAndReturn(
		func(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			return x.GetSchema(ctx, &sdcpb.GetSchemaRequest{
				Path:   path,
				Schema: sdcpbSchema,
			})
		},
	)

	tsc := NewTreeSchemaCacheClient("testds", nil, mockscb)

	// First call should result in 2 calls to GetSchema
	path, err := tsc.ToPath(ctx, []string{"network-instance", "default", "admin-state"})
	if err != nil {
		t.Error(err)
	}

	cmpPath := &sdcpb.Path{Elem: []*sdcpb.PathElem{
		{Name: "network-instance", Key: map[string]string{"name": "default"}},
		{Name: "admin-state"}},
	}

	if diff := cmp.Diff(path.String(), cmpPath.String()); diff != "" {
		t.Errorf("TreeSchemaCacheClientImpl.ToPath() mismatch (-want +got):\n%s", diff)
	}

	// second call should result in no additional calls to GetSchema
	path, err = tsc.ToPath(ctx, []string{"network-instance", "someother", "admin-state"})
	if err != nil {
		t.Error(err)
	}

	cmpPath = &sdcpb.Path{Elem: []*sdcpb.PathElem{
		{Name: "network-instance", Key: map[string]string{"name": "someother"}},
		{Name: "admin-state"}},
	}

	if diff := cmp.Diff(path.String(), cmpPath.String()); diff != "" {
		t.Errorf("TreeSchemaCacheClientImpl.ToPath() mismatch (-want +got):\n%s", diff)
	}

	// third call should result in a single additional call to GetSchema
	path, err = tsc.ToPath(ctx, []string{"network-instance", "other", "type"})
	if err != nil {
		t.Error(err)
	}

	cmpPath = &sdcpb.Path{Elem: []*sdcpb.PathElem{
		{Name: "network-instance", Key: map[string]string{"name": "other"}},
		{Name: "type"}},
	}

	if diff := cmp.Diff(path.String(), cmpPath.String()); diff != "" {
		t.Errorf("TreeSchemaCacheClientImpl.ToPath() mismatch (-want +got):\n%s", diff)
	}
}
