package datastore

import (
	"context"
	"runtime"
	"testing"

	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/consts"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/ops"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

func TestDriftRevertScopePathsNavigate(t *testing.T) {
	ctx := context.Background()
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)
	tp := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	base := driftGatePopulateRunning(t, ctx, scb, tp, driftGateDevice())
	drifted := driftGateDeviceWithDesc("wrong-on-device")
	if _, err := base.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(drifted, consts.RunningIntentName, consts.RunningValuesPrio, false), treetypes.NewUpdateInsertFlags(), tp); err != nil {
		t.Fatal(err)
	}
	changed := ops.CollectRunningSyncChangedPaths(base.Entry)
	if len(changed) == 0 {
		t.Fatal("expected changed paths")
	}
	scopes := ops.RevertScopesFromChangedPaths(changed)
	for _, scope := range scopes {
		if _, err := ops.NavigateSdcpbPath(ctx, base.Entry, scope); err != nil {
			t.Fatalf("scope %v navigate: %v", scope.ToXPath(false), err)
		}
	}

	intent := driftGateIntentExport(t, ctx, tp, scb, driftGateDevice())
	revertTree, err := buildPartialRevertTree(ctx, base, scopes)
	if err != nil {
		t.Fatal(err)
	}
	ds := driftGateDatastore(t, gomock.NewController(t), scb, base, nil, intent)
	if err := ds.loadIntentsForScopes(ctx, revertTree, scopes); err != nil {
		t.Fatal(err)
	}
	if err := revertTree.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}
	upd, err := ops.ToProtoUpdates(ctx, revertTree.Entry, true)
	if err != nil {
		t.Fatal(err)
	}
	updAll, err := ops.ToProtoUpdates(ctx, revertTree.Entry, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("updates onlyNewOrUpdated=%d all=%d", len(upd), len(updAll))
	if len(upd) == 0 && len(updAll) == 0 {
		t.Fatal("expected drift updates from partial revert tree")
	}
}

func TestDriftRevertNonRevertivePartialCompare(t *testing.T) {
	ctx := context.Background()
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)
	tp := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	base := driftGatePopulateRunning(t, ctx, scb, tp, driftGateDevice())
	drifted := driftGateDeviceWithDesc("wrong-on-device")
	if _, err := base.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(drifted, consts.RunningIntentName, consts.RunningValuesPrio, false), treetypes.NewUpdateInsertFlags(), tp); err != nil {
		t.Fatal(err)
	}
	scopes := ops.RevertScopesFromChangedPaths(ops.CollectRunningSyncChangedPaths(base.Entry))
	intent := driftGateIntentExport(t, ctx, tp, scb, driftGateDevice())
	intent.NonRevertive = true
	revertTree, err := buildPartialRevertTree(ctx, base, scopes)
	if err != nil {
		t.Fatal(err)
	}
	ds := driftGateDatastore(t, gomock.NewController(t), scb, base, nil, intent)
	if err := ds.loadIntentsForScopes(ctx, revertTree, scopes); err != nil {
		t.Fatal(err)
	}
	if err := revertTree.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}
	upd, err := ops.ToProtoUpdates(ctx, revertTree.Entry, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range upd {
		t.Logf("unexpected update: %s", u.GetPath().ToXPath(false))
	}
	if len(upd) != 0 {
		t.Fatalf("non-revertive partial compare: want 0 updates, got %d", len(upd))
	}
}
