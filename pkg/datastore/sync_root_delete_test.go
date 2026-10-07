package datastore

import (
	"context"
	"runtime"
	"testing"

	"github.com/sdcio/data-server/mocks/mocktarget"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// TestApplyToRunningDeleteRootRemovesRunning syncs a delete of the whole configuration. The root is not a
// deletable branch of its own, its entries have to be removed one by one, otherwise the stale entries stay in
// the sync tree after the delete flags have been reset.
func TestApplyToRunningDeleteRootRemovesRunning(t *testing.T) {
	ctx := context.Background()
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)
	tp := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

	ctrl := gomock.NewController(t)
	sbi := mocktarget.NewMockTarget(ctrl)
	sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Times(0)
	ds := driftGateDatastore(t, ctrl, scb, driftGatePopulateRunning(t, ctx, scb, tp, driftGateDevice()), sbi)

	if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, nil); err != nil {
		t.Fatal(err)
	}

	left, err := ops.ToProtoUpdates(ctx, ds.syncTree.Entry, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range left {
		t.Logf("stale entry in the sync tree: %s", u.GetPath().ToXPath(false))
	}
	if len(left) != 0 {
		t.Fatalf("sync tree still holds %d entries after deleting the root", len(left))
	}
}
