package datastore

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/mocks/mockcacheclient"
	"github.com/sdcio/data-server/mocks/mocktarget"
	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/cache/configserver"
	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/datastore/types"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/importer"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	treeproto "github.com/sdcio/data-server/pkg/tree/importer/proto"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/processors"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
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
				DoAndReturn(func(ctx context.Context, excludeIntentNames []string, intentChan chan<- importer.ImportConfigAdapter, errChan chan<- error) {
					close(intentChan)
					close(errChan)
				}).AnyTimes()

			// Expect IntentModify (called by TransactionSet to save intent)
			ccb.EXPECT().IntentModify(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			// Expect RunningModify (called by writeBackSyncTree to persist running)
			ccb.EXPECT().RunningModify(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

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
				syncTreeMutex:      &sync.RWMutex{},
				syncTree:           syncTreeRoot, // Pre-populated syncTree
				taskPool:           vpf,
				cacheClient:        ccb,
				sbi:                sbi,
				dmutex:             &sync.Mutex{},
				schemaClient:       scb,
				sensitivePathIndex: treetypes.NewSensitivePathIndex(),
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

// TestTransactionSet_SensitivePathsPersisted verifies that sensitive_paths set
// on a TransactionIntent are written to the tree_persist.Intent passed to
// IntentModify (the cache write). This is the tracer bullet for Issue 03.
func TestTransactionSet_SensitivePathsPersisted(t *testing.T) {
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	wantPaths := []*sdcpb.Path{
		{Elem: []*sdcpb.PathElem{{Name: "interface"}, {Name: "description"}}, IsRootBased: true},
		{Elem: []*sdcpb.PathElem{{Name: "bgp"}, {Name: "neighbors"}, {Name: "auth-password"}}, IsRootBased: true},
	}

	// Build a minimal intent payload with one leaf so TreeExport produces
	// a non-empty tree_persist.Intent and IntentModify is called (not IntentDelete).
	intentDevice := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name:        ygot.String("ethernet-1/1"),
				Description: ygot.String("sensitive-test"),
			},
		},
	}
	intentJSON, err := ygot.EmitJSON(intentDevice, &ygot.EmitJSONConfig{
		Format:         ygot.RFC7951,
		SkipValidation: false,
	})
	if err != nil {
		t.Fatalf("failed to marshal intent: %v", err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var capturedIntent *tree_persist.Intent
	ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
	ccb.EXPECT().
		IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, excludeIntentNames []string, intentChan chan<- importer.ImportConfigAdapter, errChan chan<- error) {
			close(intentChan)
			close(errChan)
		}).AnyTimes()
	ccb.EXPECT().
		IntentModify(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, intent *tree_persist.Intent) error {
			if intent.GetIntentName() == "intent-sensitive" {
				capturedIntent = intent
			}
			return nil
		}).AnyTimes()
	ccb.EXPECT().
		RunningModify(gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	sbi := mocktarget.NewMockTarget(ctrl)
	sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).AnyTimes()

	vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	tc := tree.NewTreeContext(scb, vpf)
	syncTreeRoot, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}
	err = syncTreeRoot.FinishInsertionPhase(ctx)
	if err != nil {
		t.Fatal(err)
	}

	ds := &Datastore{
		config: &config.DatastoreConfig{
			Validation: config.NewValidationConfig(),
			Name:       "test-ds",
		},
		syncTreeMutex:      &sync.RWMutex{},
		syncTree:           syncTreeRoot,
		taskPool:           vpf,
		cacheClient:        ccb,
		sbi:                sbi,
		dmutex:             &sync.Mutex{},
		schemaClient:       scb,
		sensitivePathIndex: treetypes.NewSensitivePathIndex(),
	}
	ds.transactionManager = types.NewTransactionManager(NewDatastoreRollbackAdapter(ds))

	ti := types.NewTransactionIntent("intent-sensitive", 10)
	ti.SetSensitivePaths(wantPaths)

	updates, err := treetypes.ExpandAndConvertIntent(ctx, scb, "intent-sensitive", 10, []*sdcpb.Update{{
		Path: &sdcpb.Path{},
		Value: &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(intentJSON)},
		},
	}}, time.Now().Unix())
	if err != nil {
		t.Fatalf("failed to expand intent: %v", err)
	}
	ti.AddUpdates(updates)

	_, err = ds.TransactionSet(ctx, "txn-sensitive", []*types.TransactionIntent{ti}, nil, 10*time.Second, false)
	if err != nil {
		t.Fatalf("TransactionSet failed: %v", err)
	}

	if capturedIntent == nil {
		t.Fatal("IntentModify was not called for intent-sensitive")
	}
	gotXPaths := make([]string, 0, len(capturedIntent.GetSensitivePaths()))
	for _, p := range capturedIntent.GetSensitivePaths() {
		gotXPaths = append(gotXPaths, p.ToXPath(true))
	}
	wantXPaths := make([]string, 0, len(wantPaths))
	for _, p := range wantPaths {
		wantXPaths = append(wantXPaths, p.ToXPath(true))
	}
	if diff := cmp.Diff(wantXPaths, gotXPaths); diff != "" {
		t.Errorf("SensitivePaths mismatch (-want +got):\n%s", diff)
	}
}

// buildFixtureIntent runs a device fixture through a scratch tree exactly
// the way a real apply would (ImportConfig + FinishInsertionPhase +
// TreeExport), so tests get a *tree_persist.Intent fixture in the same shape
// the real pipeline produces, rather than a hand-built one that risks
// drifting from what TreeExport actually emits.
func buildFixtureIntent(t *testing.T, scb schemaClient.SchemaClientBound, name string, priority int32, device *sdcio_schema.Device) *tree_persist.Intent {
	t.Helper()
	return buildFixtureIntentWithValidation(t, scb, name, priority, device, false)
}

// buildFixtureIntentAllowInvalid is buildFixtureIntent, except it skips
// ygot's own leafref validation on marshal. It exists for fixtures that
// deliberately contain a dangling leafref (e.g. ghost-intent tests) — ygot's
// validation is stricter than (and duplicates) what pkg/tree/ops/validation
// exercises, and would otherwise reject such fixtures before the tree ever
// sees them. Regular fixtures should keep using buildFixtureIntent so ygot's
// own checks still catch accidentally-invalid test data.
func buildFixtureIntentAllowInvalid(t *testing.T, scb schemaClient.SchemaClientBound, name string, priority int32, device *sdcio_schema.Device) *tree_persist.Intent {
	t.Helper()
	return buildFixtureIntentWithValidation(t, scb, name, priority, device, true)
}

func buildFixtureIntentWithValidation(t *testing.T, scb schemaClient.SchemaClientBound, name string, priority int32, device *sdcio_schema.Device, skipValidation bool) *tree_persist.Intent {
	t.Helper()
	ctx := context.Background()

	deviceJSON, err := ygot.EmitJSON(device, &ygot.EmitJSONConfig{
		Format:         ygot.RFC7951,
		SkipValidation: skipValidation,
	})
	if err != nil {
		t.Fatalf("marshal fixture device: %v", err)
	}
	var contentAny any
	if err := json.Unmarshal([]byte(deviceJSON), &contentAny); err != nil {
		t.Fatalf("unmarshal fixture content: %v", err)
	}

	vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	tc := tree.NewTreeContext(scb, vpf)
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatalf("failed to create fixture tree root: %v", err)
	}
	_, err = root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(contentAny, name, priority, false), treetypes.NewUpdateInsertFlags(), vpf)
	if err != nil {
		t.Fatalf("failed to import fixture content: %v", err)
	}
	if err := root.FinishInsertionPhase(ctx); err != nil {
		t.Fatalf("failed to finish fixture insertion phase: %v", err)
	}
	intent, err := ops.TreeExport(root.Entry, name, priority, false)
	if err != nil {
		t.Fatalf("failed to export fixture intent: %v", err)
	}
	return intent
}

// TestTransactionSet_IntentDeleteFailureHardFailsTransaction is the
// regression test for the asymmetry ADR 0003 fixes: a failed IntentDelete
// RPC must hard-fail the transaction the same way a failed IntentModify
// already does, rather than being logged and swallowed — a swallowed
// failure here leaves exactly the silent ghost last-applied entry this
// whole fix exists to close.
func TestTransactionSet_IntentDeleteFailureHardFailsTransaction(t *testing.T) {
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	fixtureDevice := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name:        ygot.String("ethernet-1/1"),
				Description: ygot.String("existing"),
			},
		},
	}
	fixtureIntent := buildFixtureIntent(t, scb, "intent1", 10, fixtureDevice)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wantErr := errors.New("delete rpc failed")
	ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
	ccb.EXPECT().
		IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ []string, intentChan chan<- importer.ImportConfigAdapter, errChan chan<- error) {
			intentChan <- treeproto.NewProtoTreeImporter(fixtureIntent)
			close(intentChan)
			close(errChan)
		}).AnyTimes()
	ccb.EXPECT().IntentDelete(gomock.Any(), "intent1", gomock.Any()).Return(wantErr)

	sbi := mocktarget.NewMockTarget(ctrl)
	sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).AnyTimes()

	vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	tc := tree.NewTreeContext(scb, vpf)
	syncTreeRoot, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncTreeRoot.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}

	ds := &Datastore{
		config: &config.DatastoreConfig{
			Validation: config.NewValidationConfig(),
			Name:       "test-ds",
		},
		syncTreeMutex:      &sync.RWMutex{},
		syncTree:           syncTreeRoot,
		taskPool:           vpf,
		cacheClient:        ccb,
		sbi:                sbi,
		dmutex:             &sync.Mutex{},
		schemaClient:       scb,
		sensitivePathIndex: treetypes.NewSensitivePathIndex(),
	}
	ds.transactionManager = types.NewTransactionManager(NewDatastoreRollbackAdapter(ds))

	ti := types.NewTransactionIntent("intent1", 10)
	ti.SetDeleteFlag()

	_, err = ds.TransactionSet(ctx, "txn-delete-fail", []*types.TransactionIntent{ti}, nil, 10*time.Second, false)
	if !errors.Is(err, wantErr) {
		t.Fatalf("TransactionSet() error = %v, want wrapped %v", err, wantErr)
	}
}

// TestTransactionRollback_RestoresDeletedIntent verifies a rollback (the
// same path a timeout/cancel drives via TransactionManager.Cancel) re-runs
// TransactionSet on the transaction's old intents, and that a deleted
// intent's IntentModify call during that replay carries the pre-delete
// content back — the "rollback restores it" half of ADR 0003, symmetric
// with device-state rollback.
func TestTransactionRollback_RestoresDeletedIntent(t *testing.T) {
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	fixtureDevice := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name:        ygot.String("ethernet-1/1"),
				Description: ygot.String("existing"),
			},
		},
	}
	fixtureIntent := buildFixtureIntent(t, scb, "intent1", 10, fixtureDevice)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var deleteCalls int
	var modifyCalls []*tree_persist.Intent
	ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
	ccb.EXPECT().
		IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ []string, intentChan chan<- importer.ImportConfigAdapter, errChan chan<- error) {
			intentChan <- treeproto.NewProtoTreeImporter(fixtureIntent)
			close(intentChan)
			close(errChan)
		}).AnyTimes()
	ccb.EXPECT().
		IntentDelete(gomock.Any(), "intent1", gomock.Any()).
		DoAndReturn(func(context.Context, string, bool) error {
			deleteCalls++
			return nil
		}).AnyTimes()
	ccb.EXPECT().
		IntentModify(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, intent *tree_persist.Intent) error {
			modifyCalls = append(modifyCalls, intent)
			return nil
		}).AnyTimes()
	ccb.EXPECT().RunningModify(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sbi := mocktarget.NewMockTarget(ctrl)
	sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).AnyTimes()

	vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	tc := tree.NewTreeContext(scb, vpf)
	syncTreeRoot, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncTreeRoot.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}

	ds := &Datastore{
		config: &config.DatastoreConfig{
			Validation: config.NewValidationConfig(),
			Name:       "test-ds",
		},
		syncTreeMutex:      &sync.RWMutex{},
		syncTree:           syncTreeRoot,
		taskPool:           vpf,
		cacheClient:        ccb,
		sbi:                sbi,
		dmutex:             &sync.Mutex{},
		schemaClient:       scb,
		sensitivePathIndex: treetypes.NewSensitivePathIndex(),
	}
	ds.transactionManager = types.NewTransactionManager(NewDatastoreRollbackAdapter(ds))

	ti := types.NewTransactionIntent("intent1", 10)
	ti.SetDeleteFlag()

	_, err = ds.TransactionSet(ctx, "txn-rollback", []*types.TransactionIntent{ti}, nil, 10*time.Second, false)
	if err != nil {
		t.Fatalf("TransactionSet() (delete) error = %v", err)
	}
	if deleteCalls != 1 {
		t.Fatalf("IntentDelete call count = %d, want 1", deleteCalls)
	}
	if len(modifyCalls) != 0 {
		t.Fatalf("IntentModify call count after delete = %d, want 0", len(modifyCalls))
	}

	if err := ds.transactionManager.Cancel(ctx, "txn-rollback"); err != nil {
		t.Fatalf("Cancel() (rollback) error = %v", err)
	}

	if len(modifyCalls) != 1 {
		t.Fatalf("IntentModify call count after rollback = %d, want 1 (restore)", len(modifyCalls))
	}
	restored := modifyCalls[0]
	if got := restored.GetIntentName(); got != "intent1" {
		t.Errorf("restored intent name = %q, want %q", got, "intent1")
	}
	if got := findLeafStringValue(t, restored.GetRoot(), "description"); got != "existing" {
		t.Errorf("restored intent's description leaf = %q, want %q (the pre-delete content, not just the name)", got, "existing")
	}
}

// findLeafStringValue depth-first searches el for a leaf named name and
// returns its decoded string value, so rollback/restore tests can assert on
// actual content rather than just the intent's name/priority.
func findLeafStringValue(t *testing.T, el *tree_persist.TreeElement, name string) string {
	t.Helper()
	if el == nil {
		return ""
	}
	if el.GetName() == name && len(el.GetLeafVariant()) > 0 {
		tv := &sdcpb.TypedValue{}
		if err := proto.Unmarshal(el.GetLeafVariant(), tv); err != nil {
			t.Fatalf("unmarshal leaf %q: %v", name, err)
		}
		return tv.GetStringVal()
	}
	for _, c := range el.GetChilds() {
		if v := findLeafStringValue(t, c, name); v != "" {
			return v
		}
	}
	return ""
}

// TestConfigServerBackend_DeleteApply_NoRehydration is the ghost-intent
// regression test named directly in ticket 03's checklist: under
// Cache.Type: config-server, a delete-apply must remove the last-applied
// entry immediately, so the very next LoadAllButRunningIntents — the exact
// call that rehydrated a deleted intent1 in the original bug — does not
// bring it back. Unlike the other tests in this file, this one wires a real
// *cache.ConfigServerCache over configserver.FakeLocalConfigClient (not a
// generic mockcacheclient), so it exercises the actual seam this ticket
// built, not just "some IntentWriter got called."
func TestConfigServerBackend_DeleteApply_NoRehydration(t *testing.T) {
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	fakeClient := configserver.NewFakeLocalConfigClient()
	const cacheName = "ns1.target1"
	ccb := cache.NewCacheClientBound(cacheName, cache.NewConfigServerClient(fakeClient))
	if err := ccb.InstanceCreate(ctx); err != nil {
		t.Fatalf("InstanceCreate() error = %v", err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	sbi := mocktarget.NewMockTarget(ctrl)
	sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).AnyTimes()

	vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	tc := tree.NewTreeContext(scb, vpf)
	syncTreeRoot, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncTreeRoot.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}

	ds := &Datastore{
		config: &config.DatastoreConfig{
			Validation: config.NewValidationConfig(),
			Name:       "test-ds",
		},
		syncTreeMutex:      &sync.RWMutex{},
		syncTree:           syncTreeRoot,
		taskPool:           vpf,
		cacheClient:        ccb,
		sbi:                sbi,
		dmutex:             &sync.Mutex{},
		schemaClient:       scb,
		sensitivePathIndex: treetypes.NewSensitivePathIndex(),
	}
	ds.transactionManager = types.NewTransactionManager(NewDatastoreRollbackAdapter(ds))

	// Apply #1: create intent1 with real content.
	intentJSON, err := ygot.EmitJSON(&sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {Name: ygot.String("ethernet-1/1"), Description: ygot.String("existing")},
		},
	}, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: false})
	if err != nil {
		t.Fatalf("marshal intent content: %v", err)
	}
	tiCreate := types.NewTransactionIntent("intent1", 10)
	updates, err := treetypes.ExpandAndConvertIntent(ctx, scb, "intent1", 10, []*sdcpb.Update{{
		Path:  &sdcpb.Path{},
		Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(intentJSON)}},
	}}, time.Now().Unix())
	if err != nil {
		t.Fatalf("expand intent: %v", err)
	}
	tiCreate.AddUpdates(updates)
	if _, err := ds.TransactionSet(ctx, "txn-create", []*types.TransactionIntent{tiCreate}, nil, 10*time.Second, false); err != nil {
		t.Fatalf("TransactionSet() (create) error = %v", err)
	}
	if err := ds.transactionManager.Confirm("txn-create"); err != nil {
		t.Fatalf("Confirm() (create) error = %v", err)
	}

	// Sanity: intent1 is now visible via the seam.
	preRoot, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}
	if err := preRoot.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}
	names, err := ds.LoadAllButRunningIntents(ctx, preRoot)
	if err != nil {
		t.Fatalf("LoadAllButRunningIntents() (pre-delete) error = %v", err)
	}
	if diff := cmp.Diff([]string{"ns1.intent1"}, names); diff != "" {
		t.Fatalf("LoadAllButRunningIntents() (pre-delete) mismatch (-want +got):\n%s", diff)
	}

	// Apply #2: delete intent1.
	tiDelete := types.NewTransactionIntent("intent1", 10)
	tiDelete.SetDeleteFlag()
	if _, err := ds.TransactionSet(ctx, "txn-delete", []*types.TransactionIntent{tiDelete}, nil, 10*time.Second, false); err != nil {
		t.Fatalf("TransactionSet() (delete) error = %v", err)
	}

	// The regression: the very next LoadAllButRunningIntents must not
	// rehydrate intent1.
	postRoot, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}
	if err := postRoot.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}
	names, err = ds.LoadAllButRunningIntents(ctx, postRoot)
	if err != nil {
		t.Fatalf("LoadAllButRunningIntents() (post-delete) error = %v", err)
	}
	if len(names) != 0 {
		t.Errorf("LoadAllButRunningIntents() (post-delete) = %v, want empty — intent1 was rehydrated after delete-apply", names)
	}
}

// TestTransactionSet_ValidationError_OwnedByRPCIntent_StillFails verifies
// that a validation error on an intent submitted as part of this RPC
// hard-fails the transaction — validation errors always block apply,
// regardless of which intent owns them.
func TestTransactionSet_ValidationError_OwnedByRPCIntent_StillFails(t *testing.T) {
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
	ccb.EXPECT().
		IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ []string, intentChan chan<- importer.ImportConfigAdapter, errChan chan<- error) {
			close(intentChan)
			close(errChan)
		}).AnyTimes()

	sbi := mocktarget.NewMockTarget(ctrl)

	vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	tc := tree.NewTreeContext(scb, vpf)
	syncTreeRoot, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncTreeRoot.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}

	ds := &Datastore{
		config: &config.DatastoreConfig{
			Validation: config.NewValidationConfig(),
			Name:       "test-ds",
		},
		syncTreeMutex:      &sync.RWMutex{},
		syncTree:           syncTreeRoot,
		taskPool:           vpf,
		cacheClient:        ccb,
		sbi:                sbi,
		dmutex:             &sync.Mutex{},
		schemaClient:       scb,
		sensitivePathIndex: treetypes.NewSensitivePathIndex(),
	}
	ds.transactionManager = types.NewTransactionManager(NewDatastoreRollbackAdapter(ds))

	// intent1 is submitted directly as part of this RPC, with the same
	// dangling leafref as the ghost scenario above.
	ti := types.NewTransactionIntent("intent1", 10)
	updates, err := treetypes.ExpandAndConvertIntent(ctx, scb, "intent1", 10, []*sdcpb.Update{{
		Path: &sdcpb.Path{},
		Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: mustMarshalDevice(t, &sdcio_schema.Device{
			NetworkInstance: map[string]*sdcio_schema.SdcioModel_NetworkInstance{
				"ni1": {
					Name: ygot.String("ni1"),
					Type: sdcio_schema.SdcioModelNi_NiType_default,
					Interface: map[string]*sdcio_schema.SdcioModel_NetworkInstance_Interface{
						"ethernet-1/1": {
							Name: ygot.String("ethernet-1/1"),
							InterfaceRef: &sdcio_schema.SdcioModel_NetworkInstance_Interface_InterfaceRef{
								Interface:    ygot.String("ethernet-1/1"),
								Subinterface: ygot.Uint32(5),
							},
						},
					},
				},
			},
		})}},
	}}, time.Now().Unix())
	if err != nil {
		t.Fatalf("expand intent: %v", err)
	}
	ti.AddUpdates(updates)

	// No Set() expectation is registered on sbi: apply must not be reached
	// when the RPC's own intent fails validation, so any call would fail
	// the mock's strict expectations.
	resp, err := ds.TransactionSet(ctx, "txn-rpc-owned-error", []*types.TransactionIntent{ti}, nil, 10*time.Second, false)
	if err != nil {
		t.Fatalf("TransactionSet() error = %v, want nil (validation errors are carried in the response)", err)
	}
	if errs := resp.GetIntents()["intent1"].GetErrors(); len(errs) == 0 {
		t.Errorf("response.Intents[intent1].Errors = empty, want the dangling leafref error (RPC-owned errors must still be reported and block apply)")
	}
}

// mustMarshalDevice marshals device to RFC7951 JSON bytes, failing the test
// on error.
func mustMarshalDevice(t *testing.T, device *sdcio_schema.Device) []byte {
	t.Helper()
	b, err := ygot.EmitJSON(device, &ygot.EmitJSONConfig{
		Format:         ygot.RFC7951,
		SkipValidation: true,
	})
	if err != nil {
		t.Fatalf("marshal device: %v", err)
	}
	return []byte(b)
}

// TestForEachIntent_NarrowIntentReader verifies forEachIntent streams every
// intent from cc and invokes fn for each one. It is built against a
// MockBoundIntentReader — not the full MockCacheClientBound — since
// forEachIntent only ever reads real Intents. Satisfying this test with the
// narrower mock is the seam for Issue 03's IntentReader split.
func TestForEachIntent_NarrowIntentReader(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	intentA := &tree_persist.Intent{IntentName: "intent-a", Priority: 10}
	intentB := &tree_persist.Intent{IntentName: "intent-b", Priority: 20}

	reader := mockcacheclient.NewMockBoundIntentReader(ctrl)
	reader.EXPECT().
		IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ []string, intentChan chan<- importer.ImportConfigAdapter, errChan chan<- error) {
			intentChan <- treeproto.NewProtoTreeImporter(intentA)
			intentChan <- treeproto.NewProtoTreeImporter(intentB)
			close(intentChan)
			close(errChan)
		})

	var gotNames []string
	err := forEachIntent(ctx, reader, nil, func(intent importer.ImportConfigAdapter) error {
		gotNames = append(gotNames, intent.GetName())
		return nil
	})
	if err != nil {
		t.Fatalf("forEachIntent() error = %v", err)
	}

	wantNames := []string{"intent-a", "intent-b"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Errorf("visited intent names mismatch (-want +got):\n%s", diff)
	}
}

// TestForEachIntent_PropagatesStreamError verifies forEachIntent returns the
// first error received on the cache's error channel, again using only a
// MockBoundIntentReader.
func TestForEachIntent_PropagatesStreamError(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wantErr := errors.New("boom")

	reader := mockcacheclient.NewMockBoundIntentReader(ctrl)
	reader.EXPECT().
		IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ []string, intentChan chan<- importer.ImportConfigAdapter, errChan chan<- error) {
			errChan <- wantErr
			close(intentChan)
			close(errChan)
		})

	err := forEachIntent(ctx, reader, nil, func(importer.ImportConfigAdapter) error {
		t.Fatal("fn should not be called when the stream errors")
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("forEachIntent() error = %v, want %v", err, wantErr)
	}
}

// TestSdcpbTransactionIntentToInternalTI_SensitivePaths verifies that
// sensitive_paths schema.Path values are passed through to the internal
// TransactionIntent unchanged.
func TestSdcpbTransactionIntentToInternalTI_SensitivePaths(t *testing.T) {
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)
	ds := &Datastore{schemaClient: scb}

	paths := []*sdcpb.Path{
		{Elem: []*sdcpb.PathElem{{Name: "bgp"}, {Name: "neighbors"}, {Name: "auth-password"}}, IsRootBased: true},
		{Elem: []*sdcpb.PathElem{{Name: "interface"}, {Name: "description"}}, IsRootBased: true},
	}
	req := &sdcpb.TransactionIntent{
		Intent:         "test-intent",
		Priority:       10,
		Delete:         true,
		SensitivePaths: paths,
	}

	ti, err := ds.SdcpbTransactionIntentToInternalTI(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := ti.GetSensitivePaths()
	if len(got) != len(paths) {
		t.Fatalf("got %d sensitive paths, want %d", len(got), len(paths))
	}
	for i, p := range paths {
		if got[i].ToXPath(true) != p.ToXPath(true) {
			t.Errorf("sensitive_paths[%d]: got %q, want %q", i, got[i].ToXPath(true), p.ToXPath(true))
		}
	}
}
