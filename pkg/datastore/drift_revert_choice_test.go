package datastore

import (
	"context"
	"runtime"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/mocks/mocktarget"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/consts"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// TestDriftRevertChoiceScopeReachesChoiceOwner syncs an update of a value that sits in a case other than the
// one the intent selects. The updated value is the only touched entry, so the revert scope starts there. It
// has to reach the container that owns the choice, otherwise the intent of the intended case is never loaded
// and the drift stays uncorrected.
//
// Only an update of an existing value yields a narrow scope. A sync that merely adds entries touches
// nothing and falls back to checking the whole tree.
func TestDriftRevertChoiceScopeReachesChoiceOwner(t *testing.T) {
	ctx := context.Background()
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)
	tp := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

	caseElem := func(v string) *sdcio_schema.SdcioModel_Choices {
		return &sdcio_schema.SdcioModel_Choices{Case1: &sdcio_schema.SdcioModel_Choices_Case1{
			CaseElem: &sdcio_schema.SdcioModel_Choices_Case1_CaseElem{Elem: ygot.String(v)},
		}}
	}
	caseLog := func(v bool) *sdcio_schema.SdcioModel_Choices {
		return &sdcio_schema.SdcioModel_Choices{Case2: &sdcio_schema.SdcioModel_Choices_Case2{Log: ygot.Bool(v)}}
	}

	tests := map[string]struct {
		intended, running, tick *sdcio_schema.SdcioModel_Choices
	}{
		// the intended case holds a nested container, the touched value is a leaf of the other case
		"leaf of other case": {intended: caseElem("intended"), running: caseLog(false), tick: caseLog(true)},
		// the touched value is below a nested container of the other case
		"nested leaf of other case": {intended: caseLog(true), running: caseElem("a"), tick: caseElem("b")},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			intent := driftGateIntentExport(t, ctx, tp, scb, driftChoiceDevice(t, tt.intended))

			ctrl := gomock.NewController(t)
			sbi := mocktarget.NewMockTarget(ctrl)
			sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
			ds := driftGateDatastore(t, ctrl, scb, driftGatePopulateRunning(t, ctx, scb, tp, driftChoiceDevice(t, tt.running)), sbi, intent)

			// incremental sync, nothing is deleted
			tick := driftChoiceDevice(t, tt.tick)
			if err := ds.ApplyToRunning(ctx, nil, jsonImporter.NewJsonTreeImporter(tick, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
				t.Fatal(err)
			}
		})
	}
}
