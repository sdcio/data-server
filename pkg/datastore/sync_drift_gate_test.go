package datastore

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/mocks/mockcacheclient"
	"github.com/sdcio/data-server/mocks/mocktarget"
	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/datastore/target"
	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/ops"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	"go.uber.org/mock/gomock"
)

func driftGateDeviceWithDesc(desc string, extraIfaces ...*sdcio_schema.SdcioModel_Interface) any {
	d := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name:        ygot.String("ethernet-1/1"),
				Description: ygot.String(desc),
			},
		},
	}
	for _, itf := range extraIfaces {
		d.Interface[*itf.Name] = itf
	}
	s, err := ygot.EmitJSON(d, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: true})
	if err != nil {
		panic(err)
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		panic(err)
	}
	return v
}

func driftGateDevice(extraIfaces ...*sdcio_schema.SdcioModel_Interface) any {
	return driftGateDeviceWithDesc("intended-desc", extraIfaces...)
}

func driftGatePopulateRunning(t *testing.T, ctx context.Context, scb schemaClient.SchemaClientBound, tp *pool.SharedTaskPool, device any) *tree.RootEntry {
	t.Helper()
	tc := tree.NewTreeContext(scb, tp)
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(device, consts.RunningIntentName, consts.RunningValuesPrio, false), treetypes.NewUpdateInsertFlags(), tp)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func driftGateDatastore(t *testing.T, ctrl *gomock.Controller, scb schemaClient.SchemaClientBound, syncTree *tree.RootEntry, sbi target.Target, intents ...*tree_persist.Intent) *Datastore {
	t.Helper()
	ctx := context.Background()
	ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
	ccb.EXPECT().
		IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
			for _, i := range intents {
				intentChan <- i
			}
			close(intentChan)
			close(errChan)
		}).AnyTimes()
	return &Datastore{
		config: &config.DatastoreConfig{
			Name:       "drift-gate-test",
			Validation: config.NewValidationConfig(),
		},
		syncTreeMutex: &sync.RWMutex{},
		syncTree:      syncTree,
		taskPool:      pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)),
		cacheClient:   ccb,
		sbi:           sbi,
		dmutex:        &sync.Mutex{},
		schemaClient:  scb,
	}
}

func driftGateIntentExport(t *testing.T, ctx context.Context, tp *pool.SharedTaskPool, scb schemaClient.SchemaClientBound, device any) *tree_persist.Intent {
	t.Helper()
	tc := tree.NewTreeContext(scb, tp)
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(device, benchIntentName, benchIntentPrio, false), treetypes.NewUpdateInsertFlags(), tp)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := ops.TreeExport(root.Entry, benchIntentName, benchIntentPrio, false)
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func TestDriftRevertCoarseGate(t *testing.T) {
	ctx := context.Background()
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)
	tp := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

	base := driftGateDevice()
	intent := driftGateIntentExport(t, ctx, tp, scb, base)

	t.Run("steady state with and without intents skips target", func(t *testing.T) {
		for _, intents := range [][]*tree_persist.Intent{nil, {intent}} {
			ctrl := gomock.NewController(t)
			sbi := mocktarget.NewMockTarget(ctrl)
			sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Times(0)
			ds := driftGateDatastore(t, ctrl, scb, driftGatePopulateRunning(t, ctx, scb, tp, base), sbi, intents...)
			if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(base, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("unmanaged config added by hand does not call target", func(t *testing.T) {
		withExtra := driftGateDevice(&sdcio_schema.SdcioModel_Interface{
			Name:        ygot.String("ethernet-1/2"),
			Description: ygot.String("brownfield"),
		})
		ctrl := gomock.NewController(t)
		sbi := mocktarget.NewMockTarget(ctrl)
		sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Times(0)
		ds := driftGateDatastore(t, ctrl, scb, driftGatePopulateRunning(t, ctx, scb, tp, base), sbi, intent)
		if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(withExtra, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("unmanaged config under delete-path intent triggers delete on target", func(t *testing.T) {
		withExtra := driftGateDevice(&sdcio_schema.SdcioModel_Interface{
			Name:        ygot.String("ethernet-1/2"),
			Description: ygot.String("brownfield"),
		})
		deleteIntent := &tree_persist.Intent{
			IntentName:      "delete-root",
			Priority:        consts.RunningValuesPrio - 1,
			ExplicitDeletes: []*sdcpb.Path{{}},
		}
		ctrl := gomock.NewController(t)
		sbi := mocktarget.NewMockTarget(ctrl)
		sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
		ds := driftGateDatastore(t, ctrl, scb, driftGatePopulateRunning(t, ctx, scb, tp, base), sbi, deleteIntent)
		if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(withExtra, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("hand change contradicting revertive intent calls target", func(t *testing.T) {
		drifted := driftGateDeviceWithDesc("wrong-on-device")

		ctrl := gomock.NewController(t)
		sbi := mocktarget.NewMockTarget(ctrl)
		sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
		ds := driftGateDatastore(t, ctrl, scb, driftGatePopulateRunning(t, ctx, scb, tp, base), sbi, intent)
		if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(drifted, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("failed drift revert retries on next steady sync", func(t *testing.T) {
		var calls atomic.Int32
		ctrl := gomock.NewController(t)
		sbi := mocktarget.NewMockTarget(ctrl)
		sbi.EXPECT().Set(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
			if calls.Add(1) == 1 {
				return nil, errors.New("target unreachable")
			}
			return &sdcpb.SetDataResponse{}, nil
		}).Times(2)

		drifted := driftGateDeviceWithDesc("wrong-on-device")

		ds := driftGateDatastore(t, ctrl, scb, driftGatePopulateRunning(t, ctx, scb, tp, base), sbi, intent)
		if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(drifted, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatal(err)
		}
		if !ds.outstandingDriftRevert.Load() {
			t.Fatal("expected outstanding drift revert after failed apply")
		}
		// Steady device state (still wrong vs intent) but no further Running change.
		if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(drifted, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatal(err)
		}
		if ds.outstandingDriftRevert.Load() {
			t.Fatal("expected outstanding marker cleared after successful retry")
		}
	})

	t.Run("concurrent syncs do not drop outstanding marker on failure", func(t *testing.T) {
		var calls atomic.Int32
		ctrl := gomock.NewController(t)
		sbi := mocktarget.NewMockTarget(ctrl)
		sbi.EXPECT().Set(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
			calls.Add(1)
			return nil, errors.New("always fails")
		}).MinTimes(1)

		drifted := driftGateDeviceWithDesc("wrong-on-device")

		ds := driftGateDatastore(t, ctrl, scb, driftGatePopulateRunning(t, ctx, scb, tp, base), sbi, intent)
		if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(drifted, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatal(err)
		}

		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(drifted, consts.RunningIntentName, consts.RunningValuesPrio, false))
			}()
		}
		wg.Wait()
		if !ds.outstandingDriftRevert.Load() {
			t.Fatal("outstanding drift revert marker must remain set after concurrent failures")
		}
	})

	t.Run("nil importer steady scoped refresh skips target", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		sbi := mocktarget.NewMockTarget(ctrl)
		sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Times(0)
		ds := driftGateDatastore(t, ctrl, scb, driftGatePopulateRunning(t, ctx, scb, tp, base), sbi, intent)
		if err := ds.ApplyToRunning(ctx, nil, nil); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("first sync after restart evaluates full state", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		sbi := mocktarget.NewMockTarget(ctrl)
		sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).MaxTimes(1)
		tc := tree.NewTreeContext(scb, tp)
		empty, err := tree.NewTreeRoot(ctx, tc)
		if err != nil {
			t.Fatal(err)
		}
		ds := driftGateDatastore(t, ctrl, scb, empty, sbi, intent)
		if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(base, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatal(err)
		}
	})
}
