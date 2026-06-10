package datastore

import (
	"context"
	"encoding/json"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
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
	json.Unmarshal([]byte(runningJson), &runningAny)

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
		DoAndReturn(func(ctx context.Context, excludeIntentNames []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
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
