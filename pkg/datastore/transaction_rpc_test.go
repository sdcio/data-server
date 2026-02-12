package datastore

import (
	"context"
	"encoding/json"
	"runtime"
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
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
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
			_, err = syncTreeRoot.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(runningAny, tree.RunningIntentName, tree.RunningValuesPrio, false), treetypes.NewUpdateInsertFlags(), vpf)
			if err != nil {
				t.Fatalf("failed to import running config: %v", err)
			}
			err = syncTreeRoot.FinishInsertionPhase(ctx)
			if err != nil {
				t.Fatalf("failed to finish insertion phase: %v", err)
			}
			// Reset flags on syncTree so everything is "existing"
			resetFlagsProcessorParams := tree.NewResetFlagsProcessorParameters().SetNewFlag().SetUpdateFlag()
			err = tree.NewResetFlagsProcessor(resetFlagsProcessorParams).Run(syncTreeRoot.GetRoot(), vpf)
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
