package datastore

import (
	"context"
	"runtime"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/mocks/mocktarget"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/consts"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	"go.uber.org/mock/gomock"
)

func TestDriftRevertChangedPathComparison(t *testing.T) {
	ctx := context.Background()
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)
	tp := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

	base := driftGateDevice()
	intent := driftGateIntentExport(t, ctx, tp, scb, base)

	t.Run("hand change matching intended value does not call target", func(t *testing.T) {
		matching := driftGateDeviceWithDesc("intended-desc")
		ctrl := gomock.NewController(t)
		sbi := mocktarget.NewMockTarget(ctrl)
		sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Times(0)
		ds := driftGateDatastore(t, ctrl, scb, driftGatePopulateRunning(t, ctx, scb, tp, base), sbi, intent)
		if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(matching, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("hand deleted required leaf calls target", func(t *testing.T) {
		noDesc := &sdcio_schema.Device{
			Interface: map[string]*sdcio_schema.SdcioModel_Interface{
				"ethernet-1/1": {
					Name: ygot.String("ethernet-1/1"),
				},
			},
		}
		ctrl := gomock.NewController(t)
		sbi := mocktarget.NewMockTarget(ctrl)
		sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
		ds := driftGateDatastore(t, ctrl, scb, driftGatePopulateRunning(t, ctx, scb, tp, base), sbi, intent)
		if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(noDesc, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("intent value wins over delete-path at same leaf", func(t *testing.T) {
		wrong := driftGateDeviceWithDesc("wrong-on-device")
		deleteIntent := &tree_persist.Intent{
			IntentName:      "delete-root",
			Priority:        consts.RunningValuesPrio - 1,
			ExplicitDeletes: []*sdcpb.Path{{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", nil)}}},
		}
		ctrl := gomock.NewController(t)
		sbi := mocktarget.NewMockTarget(ctrl)
		sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
		ds := driftGateDatastore(t, ctrl, scb, driftGatePopulateRunning(t, ctx, scb, tp, base), sbi, intent, deleteIntent)
		if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(wrong, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("non-revertive intent hand change does not call target", func(t *testing.T) {
		nonRevIntent := driftGateIntentExport(t, ctx, tp, scb, base)
		nonRevIntent.NonRevertive = true
		wrong := driftGateDeviceWithDesc("wrong-on-device")
		ctrl := gomock.NewController(t)
		sbi := mocktarget.NewMockTarget(ctrl)
		sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Times(0)
		ds := driftGateDatastore(t, ctrl, scb, driftGatePopulateRunning(t, ctx, scb, tp, base), sbi, nonRevIntent)
		if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(wrong, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("two orchestrators keep converging on intended value", func(t *testing.T) {
		var calls int
		ctrl := gomock.NewController(t)
		sbi := mocktarget.NewMockTarget(ctrl)
		sbi.EXPECT().Set(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
			calls++
			return &sdcpb.SetDataResponse{}, nil
		}).Times(2)

		ds := driftGateDatastore(t, ctrl, scb, driftGatePopulateRunning(t, ctx, scb, tp, base), sbi, intent)
		if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(driftGateDeviceWithDesc("wrong-first"), consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatal(err)
		}
		if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(driftGateDeviceWithDesc("wrong-second"), consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatal(err)
		}
		if calls != 2 {
			t.Fatalf("expected target called on each changed drift sync, got %d", calls)
		}
	})
}
