package datastore

import (
	"context"
	"encoding/json"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/mocks/mockcacheclient"
	"github.com/sdcio/data-server/mocks/mocktarget"
	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/datastore/types"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/processors"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	"go.uber.org/mock/gomock"
)

func TestTransactionSet_PreviouslyApplied(t *testing.T) {
	ctx := context.Background()

	// Setup Schema
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	// Setup Running Config Data
	runningDevice := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name:        ygot.String("ethernet-1/1"),
				Description: ygot.String("my description"),
			},
		},
	}
	runningJson, err := ygot.EmitJSON(runningDevice, &ygot.EmitJSONConfig{
		Format:         ygot.RFC7951,
		SkipValidation: false,
	})
	if err != nil {
		t.Fatalf("failed to marshal running config: %v", err)
	}
	var runningAny any
	if err := json.Unmarshal([]byte(runningJson), &runningAny); err != nil {
		t.Fatalf("unmarshal running config: %v", err)
	}

	// Setup Intent Data (Same as Running)
	intentStrSame := runningJson // Same content

	// Setup Intent Data (Different)
	deviceDiff := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name:        ygot.String("ethernet-1/1"),
				Description: ygot.String("new description"),
			},
		},
	}
	intentStrDiff, _ := ygot.EmitJSON(deviceDiff, &ygot.EmitJSONConfig{
		Format:         ygot.RFC7951,
		SkipValidation: false,
	})

	tests := []struct {
		name              string
		previouslyApplied bool
		nonRevertive      bool
		intentStr         string
		expectUpdates     bool
	}{
		{
			name:              "Revertive - Not Previously Applied - Should Produce Updates (Redundant)",
			previouslyApplied: false,
			nonRevertive:      false,
			intentStr:         intentStrSame,
			expectUpdates:     true,
		},
		{
			name:              "Revertive - Previously Applied - Should Produce Updates (PA Ignored)",
			previouslyApplied: true,
			nonRevertive:      false,
			intentStr:         intentStrSame,
			expectUpdates:     true,
		},
		{
			name:              "Revertive - Previously Applied - But Value Changed - Should Produce Updates",
			previouslyApplied: true,
			nonRevertive:      false,
			intentStr:         intentStrDiff,
			expectUpdates:     true,
		},
		{
			name:              "NonRevertive - Previously Applied - Value Changed - Should Produce NO Updates",
			previouslyApplied: true,
			nonRevertive:      true,
			intentStr:         intentStrDiff,
			expectUpdates:     false,
		},
		{
			name:              "NonRevertive - Not Previously Applied - Value Changed - Should Produce Updates",
			previouslyApplied: false,
			nonRevertive:      true,
			intentStr:         intentStrDiff,
			expectUpdates:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Setup Mock Cache Client
			ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
			// Expect IntentGetAll (called by LoadAllButRunningIntents)
			ccb.EXPECT().
				IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, excludeIntentNames []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
					close(intentChan)
					close(errChan)
				}).AnyTimes()

			// Expect IntentModify (called by TransactionSet to save intent)
			ccb.EXPECT().IntentModify(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			// Expect IntentGet (called once up front by replaceThenMerge to capture the
			// pre-replace/pre-merge Running snapshot). No running intent stored in the cache
			// for this test (running lives directly on the pre-populated syncTree instead).
			ccb.EXPECT().IntentGet(gomock.Any(), consts.RunningIntentName).Return(nil, nil).AnyTimes()

			// Setup Mock SBI
			sbi := mocktarget.NewMockTarget(ctrl)
			// Expect Set if updates are expected or if dryRun is false (we will use dryRun=false)
			// Actually TransactionSet calls applyIntent which calls sbi.Set
			sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).AnyTimes()

			// Setup SyncTree with Running Config
			tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
			syncTreeRoot, err := tree.NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatalf("failed to create sync tree root: %v", err)
			}
			vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
			// Populate SyncTree with Running config
			_, err = syncTreeRoot.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(runningAny, consts.RunningIntentName, consts.RunningValuesPrio, false), treetypes.NewUpdateInsertFlags(), vpf)
			if err != nil {
				t.Fatalf("failed to import running config: %v", err)
			}
			err = syncTreeRoot.FinishInsertionPhase(ctx)
			if err != nil {
				t.Fatalf("failed to finish insertion phase: %v", err)
			}
			// Reset flags on syncTree so everything is "existing"
			err = processors.NewResetFlagsProcessor(&processors.ResetFlagsProcessorParams{NewFlag: true, UpdateFlag: true}).Run(syncTreeRoot.Entry, vpf)
			if err != nil {
				t.Fatalf("failed to reset flags: %v", err)
			}

			// Setup Datastore
			ds := &Datastore{
				config: &config.DatastoreConfig{
					Validation: config.NewValidationConfig(),
					Name:       "test-ds",
				},
				syncTreeMutex: &sync.RWMutex{},
				syncTree:      syncTreeRoot, // Pre-populated syncTree
				taskPool:      vpf,
				cacheClient:   ccb,
				sbi:           sbi,
				dmutex:        &sync.Mutex{},
				schemaClient:  scb,
			}
			ds.transactionManager = types.NewTransactionManager(NewDatastoreRollbackAdapter(ds))

			// Prepare Transaction Request
			transactionId := "txn-1"

			// Build TransactionIntent from req using helper or manually
			// We need types.TransactionIntent
			intentName := "intent1"
			priority := int32(10)

			ti := types.NewTransactionIntent(intentName, priority)
			if tt.previouslyApplied {
				ti.SetPreviouslyApplied()
			}
			if tt.nonRevertive {
				ti.SetNonRevertive()
			}

			// Parse updates from intentStr
			updates, err := treetypes.ExpandAndConvertIntent(ctx, scb, intentName, priority, []*sdcpb.Update{{
				Path: &sdcpb.Path{},
				Value: &sdcpb.TypedValue{
					Value: &sdcpb.TypedValue_JsonVal{
						JsonVal: []byte(tt.intentStr),
					},
				},
			}}, time.Now().Unix())
			if err != nil {
				t.Fatalf("failed to expand intent: %v", err)
			}
			ti.AddUpdates(updates)

			transactionIntents := []*types.TransactionIntent{ti}

			// Call TransactionSet
			resp, err := ds.TransactionSet(ctx, transactionId, transactionIntents, nil, 10*time.Second, false)
			if err != nil {
				t.Fatalf("TransactionSet failed: %v", err)
			}

			// Verify Updates
			hasUpdates := len(resp.GetUpdate()) > 0
			if hasUpdates != tt.expectUpdates {
				t.Errorf("Expected updates: %v, got: %v (count: %d)\nUpdates: %v", tt.expectUpdates, hasUpdates, len(resp.GetUpdate()), resp.GetUpdate())
			}
		})
	}
}

// deviceJSON marshals the given ygot device into an RFC7951 JSON `any`, ready for
// treetypes.ExpandAndConvertIntent / jsonImporter consumption.
func deviceJSON(t *testing.T, dev *sdcio_schema.Device) (string, any) {
	t.Helper()
	str, err := ygot.EmitJSON(dev, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: false})
	if err != nil {
		t.Fatalf("failed to marshal device config: %v", err)
	}
	var out any
	if err := json.Unmarshal([]byte(str), &out); err != nil {
		t.Fatalf("failed to unmarshal device config: %v", err)
	}
	return str, out
}

// newReplaceTestDatastore builds a Datastore, backed by a syncTree pre-populated with
// originalDevice as Running, and a mocked cache client whose IntentGet(..., "running") always
// reflects the current content of that syncTree (mirroring how the real cache would behave).
func newReplaceTestDatastore(t *testing.T, ctrl *gomock.Controller, originalDevice *sdcio_schema.Device) (*Datastore, schemaClient.SchemaClientBound) {
	t.Helper()
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	_, originalAny := deviceJSON(t, originalDevice)

	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	syncTreeRoot, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatalf("failed to create sync tree root: %v", err)
	}
	vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	_, err = syncTreeRoot.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(originalAny, consts.RunningIntentName, consts.RunningValuesPrio, false), treetypes.NewUpdateInsertFlags(), vpf)
	if err != nil {
		t.Fatalf("failed to import running config: %v", err)
	}
	if err := syncTreeRoot.FinishInsertionPhase(ctx); err != nil {
		t.Fatalf("failed to finish insertion phase: %v", err)
	}
	if err := processors.NewResetFlagsProcessor(&processors.ResetFlagsProcessorParams{NewFlag: true, UpdateFlag: true}).Run(syncTreeRoot.Entry, vpf); err != nil {
		t.Fatalf("failed to reset flags: %v", err)
	}

	// storedIntents is a minimal stand-in for the persisted intended store: it tracks every
	// non-running intent modified/deleted through the mock, so that a later
	// LoadAllButRunningIntents (used by both the original transaction and any rollback of it)
	// sees the same intents a real cache would.
	storedIntents := map[string]*tree_persist.Intent{}

	ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
	ccb.EXPECT().
		IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, excludeIntentNames []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
			defer close(intentChan)
			defer close(errChan)
			for name, intent := range storedIntents {
				if slices.Contains(excludeIntentNames, name) {
					continue
				}
				intentChan <- intent
			}
		}).AnyTimes()
	ccb.EXPECT().IntentModify(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, intent *tree_persist.Intent) error {
			storedIntents[intent.GetIntentName()] = intent
			return nil
		}).AnyTimes()
	ccb.EXPECT().IntentDelete(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, intentName string, ignoreNonExisting bool) error {
			delete(storedIntents, intentName)
			return nil
		}).AnyTimes()
	// IntentGet(..., "running") always reflects whatever is currently in the syncTree, mirroring
	// how the persisted Running intent tracks writeBackSyncTree's writes in the real cache.
	ccb.EXPECT().IntentGet(gomock.Any(), consts.RunningIntentName).DoAndReturn(
		func(ctx context.Context, name string) (*tree_persist.Intent, error) {
			return ops.TreeExport(syncTreeRoot.Entry, consts.RunningIntentName, consts.RunningValuesPrio, false)
		}).AnyTimes()
	ccb.EXPECT().IntentExists(gomock.Any(), consts.RunningIntentName).DoAndReturn(
		func(ctx context.Context, name string) (bool, error) {
			_, err := ops.TreeExport(syncTreeRoot.Entry, consts.RunningIntentName, consts.RunningValuesPrio, false)
			return err == nil, nil
		}).AnyTimes()

	sbi := mocktarget.NewMockTarget(ctrl)
	sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).AnyTimes()

	ds := &Datastore{
		config: &config.DatastoreConfig{
			Validation: config.NewValidationConfig(),
			Name:       "test-ds",
		},
		syncTreeMutex: &sync.RWMutex{},
		syncTree:      syncTreeRoot,
		taskPool:      vpf,
		cacheClient:   ccb,
		sbi:           sbi,
		dmutex:        &sync.Mutex{},
		schemaClient:  scb,
	}
	ds.transactionManager = types.NewTransactionManager(NewDatastoreRollbackAdapter(ds))
	return ds, scb
}

func runningDescriptionLeafs(ds *Datastore) string {
	ds.syncTreeMutex.RLock()
	defer ds.syncTreeMutex.RUnlock()
	return ops.LeafsOfOwner(ds.syncTree.Entry, consts.RunningIntentName).String()
}

// Test: replace transaction's timeout/cancel actually restores pre-replace device state.
func TestTransactionSet_ReplaceCancelRestoresPreReplaceRunning(t *testing.T) {
	ctx := context.Background()

	originalDevice := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name:        ygot.String("ethernet-1/1"),
				Description: ygot.String("original description"),
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ds, scb := newReplaceTestDatastore(t, ctrl, originalDevice)

	if got := runningDescriptionLeafs(ds); !strings.Contains(got, "original description") {
		t.Fatalf("expected running to start with original description, got: %s", got)
	}

	// Build the replace intent content: same device, new description.
	replacedDevice := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name:        ygot.String("ethernet-1/1"),
				Description: ygot.String("replaced description"),
			},
		},
	}
	replacedJSON, _ := deviceJSON(t, replacedDevice)

	replaceUpdates, err := treetypes.ExpandAndConvertIntent(ctx, scb, consts.ReplaceIntentName, consts.ReplaceValuesPrio, []*sdcpb.Update{{
		Path: &sdcpb.Path{},
		Value: &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(replacedJSON)},
		},
	}}, time.Now().Unix())
	if err != nil {
		t.Fatalf("failed to expand replace intent: %v", err)
	}
	replaceTI := types.NewTransactionIntent(consts.ReplaceIntentName, consts.ReplaceValuesPrio)
	replaceTI.AddUpdates(replaceUpdates)

	transactionId := "txn-replace-1"
	_, err = ds.TransactionSet(ctx, transactionId, nil, replaceTI, time.Minute, false)
	if err != nil {
		t.Fatalf("TransactionSet (replace) failed: %v", err)
	}

	if got := runningDescriptionLeafs(ds); !strings.Contains(got, "replaced description") {
		t.Fatalf("expected replace to apply the new description, got: %s", got)
	}
	if got := runningDescriptionLeafs(ds); strings.Contains(got, "original description") {
		t.Fatalf("expected replace to have removed the original description, got: %s", got)
	}

	// Simulate a timeout/cancel of the still-open replace transaction: this must trigger the
	// rollback and actually restore the pre-replace device state (the bug this ticket fixes).
	if err := ds.TransactionCancel(ctx, transactionId); err != nil {
		t.Fatalf("TransactionCancel failed: %v", err)
	}

	if got := runningDescriptionLeafs(ds); !strings.Contains(got, "original description") {
		t.Fatalf("expected cancel to restore the original pre-replace description, got: %s", got)
	}
	if got := runningDescriptionLeafs(ds); strings.Contains(got, "replaced description") {
		t.Fatalf("expected cancel to remove the replaced description, got: %s", got)
	}
}

// Test: plain intents-only transaction's rollback is unaffected (still oldIntents-only, no
// replace is involved at all).
func TestTransactionSet_PlainIntentCancelUsesOldIntentsOnly(t *testing.T) {
	ctx := context.Background()

	// no description set on the original running device: intent1 (below) is the first thing to
	// ever set it, so its rollback via oldIntents (an empty "intent1 did not exist" entry) must
	// remove the description entirely, restoring the pre-intent (absent) state.
	originalDevice := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name: ygot.String("ethernet-1/1"),
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ds, scb := newReplaceTestDatastore(t, ctrl, originalDevice)

	intentDevice := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name:        ygot.String("ethernet-1/1"),
				Description: ygot.String("intent description"),
			},
		},
	}
	intentJSON, _ := deviceJSON(t, intentDevice)

	intentName := "intent1"
	priority := int32(10)
	updates, err := treetypes.ExpandAndConvertIntent(ctx, scb, intentName, priority, []*sdcpb.Update{{
		Path: &sdcpb.Path{},
		Value: &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(intentJSON)},
		},
	}}, time.Now().Unix())
	if err != nil {
		t.Fatalf("failed to expand intent: %v", err)
	}
	ti := types.NewTransactionIntent(intentName, priority)
	ti.AddUpdates(updates)

	transactionId := "txn-plain-1"
	_, err = ds.TransactionSet(ctx, transactionId, []*types.TransactionIntent{ti}, nil, time.Minute, false)
	if err != nil {
		t.Fatalf("TransactionSet failed: %v", err)
	}

	if got := runningDescriptionLeafs(ds); !strings.Contains(got, "intent description") {
		t.Fatalf("expected intent to apply, got: %s", got)
	}

	if err := ds.TransactionCancel(ctx, transactionId); err != nil {
		t.Fatalf("TransactionCancel failed: %v", err)
	}

	// The intent never existed before, so the rollback (replaying oldIntents, which is the empty
	// "intent1 did not exist" entry) must remove it again, restoring the pre-intent (absent) state.
	if got := runningDescriptionLeafs(ds); strings.Contains(got, "intent description") {
		t.Fatalf("expected cancel to remove the intent's description, got: %s", got)
	}
}

// Test: cancelling/timing out a rollback-of-a-rollback is not attempted. Rollback transactions
// don't start their own rollback timer, so a rollback-transaction is never itself subject to a
// further timeout/cancel. Confirm this remains unchanged.
func TestTransactionSet_RollbackTransactionDoesNotStartOwnRollbackTimer(t *testing.T) {
	ctx := context.Background()

	originalDevice := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name:        ygot.String("ethernet-1/1"),
				Description: ygot.String("original description"),
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ds, scb := newReplaceTestDatastore(t, ctrl, originalDevice)

	intentDevice := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name:        ygot.String("ethernet-1/1"),
				Description: ygot.String("intent description"),
			},
		},
	}
	intentJSON, _ := deviceJSON(t, intentDevice)

	intentName := "intent1"
	priority := int32(10)
	updates, err := treetypes.ExpandAndConvertIntent(ctx, scb, intentName, priority, []*sdcpb.Update{{
		Path: &sdcpb.Path{},
		Value: &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(intentJSON)},
		},
	}}, time.Now().Unix())
	if err != nil {
		t.Fatalf("failed to expand intent: %v", err)
	}
	ti := types.NewTransactionIntent(intentName, priority)
	ti.AddUpdates(updates)

	transactionId := "txn-rollback-timer-1"
	_, err = ds.TransactionSet(ctx, transactionId, []*types.TransactionIntent{ti}, nil, time.Minute, false)
	if err != nil {
		t.Fatalf("TransactionSet failed: %v", err)
	}

	if err := ds.TransactionCancel(ctx, transactionId); err != nil {
		t.Fatalf("TransactionCancel failed: %v", err)
	}

	// If the rollback transaction had started its own rollback timer, the TransactionManager
	// would still consider a transaction "ongoing" (or a background rollback-of-rollback could
	// later fire); since Cancel's cleanup already cleared it, a brand new transaction must be
	// immediately registrable. Build a fresh, differently-named TransactionIntent (rather than
	// reusing ti / intent1) since that's what a real, unrelated second request would send.
	intentName2 := "intent2"
	updates2, err := treetypes.ExpandAndConvertIntent(ctx, scb, intentName2, priority, []*sdcpb.Update{{
		Path: &sdcpb.Path{},
		Value: &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(intentJSON)},
		},
	}}, time.Now().Unix())
	if err != nil {
		t.Fatalf("failed to expand intent: %v", err)
	}
	ti2 := types.NewTransactionIntent(intentName2, priority)
	ti2.AddUpdates(updates2)

	transactionId2 := "txn-after-rollback-1"
	_, err = ds.TransactionSet(ctx, transactionId2, []*types.TransactionIntent{ti2}, nil, time.Minute, false)
	if err != nil {
		t.Fatalf("expected to be able to start a new transaction right after a rollback completed, got: %v", err)
	}
}
