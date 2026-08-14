package datastore

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/mocks/mockcacheclient"
	"github.com/sdcio/data-server/mocks/mocktarget"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/datastore/target"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/importer"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/processors"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	"go.uber.org/mock/gomock"
)

// TestApplyToRunning tests the ApplyToRunning method of the Datastore struct.
func TestApplyToRunning(t *testing.T) {
	// Setup
	ctx := context.Background()

	// Define test cases
	tests := []struct {
		name            string
		deletes         []*sdcpb.Path
		importerFunc    func() importer.ImportConfigAdapter
		syncTreeFunc    func() *tree.RootEntry
		resultFunc      func() any
		cacheClientFunc func(ctrl *gomock.Controller) *mockcacheclient.MockCacheClientBound
		sbiFunc         func(ctrl *gomock.Controller) target.Target
		wantErr         bool
	}{
		{
			name:    "delete entire interface (e1-1 existed now e1-2 added)",
			deletes: []*sdcpb.Path{{Elem: []*sdcpb.PathElem{}}},
			syncTreeFunc: func() *tree.RootEntry {

				ctx := context.Background()

				sc, schema, err := testhelper.InitSDCIOSchema()
				if err != nil {
					t.Fatal(err)
				}
				scb := schemaClient.NewSchemaClientBound(schema, sc)
				tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

				root, err := tree.NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatalf("failed to create new tree root: %v", err)
				}

				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/1": {
							Name:        ygot.String("ethernet-1/1"),
							Description: ygot.String("my description"),
						},
					},
				}
				confStr, err := ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
				if err != nil {
					t.Fatalf("failed to marshal test config: %v", err)
				}

				var v any
				json.Unmarshal([]byte(confStr), &v)

				vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
				_, err = root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(v, consts.RunningIntentName, consts.RunningValuesPrio, false), types.NewUpdateInsertFlags(), vpf)
				if err != nil {
					t.Fatalf("failed to import test config: %v", err)
				}

				return root

			},
			importerFunc: func() importer.ImportConfigAdapter {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/2": {
							Name:        ygot.String("ethernet-1/2"),
							Description: ygot.String("1/2 description"),
						},
					},
				}
				confStr, err := ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
				if err != nil {
					t.Fatalf("failed to marshal test config: %v", err)
				}

				var v any
				json.Unmarshal([]byte(confStr), &v)

				return jsonImporter.NewJsonTreeImporter(v, consts.RunningIntentName, consts.RunningValuesPrio, false)
			},
			resultFunc: func() any {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/2": {
							Name:        ygot.String("ethernet-1/2"),
							Description: ygot.String("1/2 description"),
						},
					},
				}
				confStr, err := ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
				if err != nil {
					t.Fatalf("failed to marshal test config: %v", err)
				}

				var v any
				json.Unmarshal([]byte(confStr), &v)
				return v
			},
			wantErr: false,
			cacheClientFunc: func(ctrl *gomock.Controller) *mockcacheclient.MockCacheClientBound {
				ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
				ccb.EXPECT().
					IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, excludeIntentNames []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
						close(intentChan)
						close(errChan)
					}).AnyTimes()
				return ccb
			},
			sbiFunc: func(ctrl *gomock.Controller) target.Target {
				sbi := mocktarget.NewMockTarget(ctrl)
				return sbi
			},
		},
		{
			name:    "delete description of existing interface",
			deletes: []*sdcpb.Path{{Elem: []*sdcpb.PathElem{}}},
			syncTreeFunc: func() *tree.RootEntry {

				ctx := context.Background()

				sc, schema, err := testhelper.InitSDCIOSchema()
				if err != nil {
					t.Fatal(err)
				}
				scb := schemaClient.NewSchemaClientBound(schema, sc)
				tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

				root, err := tree.NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatalf("failed to create new tree root: %v", err)
				}

				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/1": {
							Name:        ygot.String("ethernet-1/1"),
							Description: ygot.String("my description"),
						},
					},
				}
				confStr, err := ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
				if err != nil {
					t.Fatalf("failed to marshal test config: %v", err)
				}

				var v any
				json.Unmarshal([]byte(confStr), &v)

				vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
				_, err = root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(v, consts.RunningIntentName, consts.RunningValuesPrio, false), types.NewUpdateInsertFlags(), vpf)
				if err != nil {
					t.Fatalf("failed to import test config: %v", err)
				}

				return root

			},
			importerFunc: func() importer.ImportConfigAdapter {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/1": {
							Name: ygot.String("ethernet-1/1"),
						},
					},
				}
				confStr, err := ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
				if err != nil {
					t.Fatalf("failed to marshal test config: %v", err)
				}

				var v any
				json.Unmarshal([]byte(confStr), &v)

				return jsonImporter.NewJsonTreeImporter(v, consts.RunningIntentName, consts.RunningValuesPrio, false)
			},
			resultFunc: func() any {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/1": {
							Name: ygot.String("ethernet-1/1"),
						},
					},
				}
				confStr, err := ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
				if err != nil {
					t.Fatalf("failed to marshal test config: %v", err)
				}

				var v any
				json.Unmarshal([]byte(confStr), &v)
				return v
			},
			wantErr: false,
			cacheClientFunc: func(ctrl *gomock.Controller) *mockcacheclient.MockCacheClientBound {
				ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
				ccb.EXPECT().
					IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, excludeIntentNames []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
						close(intentChan)
						close(errChan)
					}).AnyTimes()
				return ccb
			},
			sbiFunc: func(ctrl *gomock.Controller) target.Target {
				sbi := mocktarget.NewMockTarget(ctrl)
				return sbi
			},
		},
		{
			name:    "change description of existing interface",
			deletes: []*sdcpb.Path{{Elem: []*sdcpb.PathElem{}}},
			syncTreeFunc: func() *tree.RootEntry {

				ctx := context.Background()

				sc, schema, err := testhelper.InitSDCIOSchema()
				if err != nil {
					t.Fatal(err)
				}
				scb := schemaClient.NewSchemaClientBound(schema, sc)
				tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

				root, err := tree.NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatalf("failed to create new tree root: %v", err)
				}

				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/1": {
							Name:        ygot.String("ethernet-1/1"),
							Description: ygot.String("my description"),
						},
					},
				}
				confStr, err := ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
				if err != nil {
					t.Fatalf("failed to marshal test config: %v", err)
				}

				var v any
				json.Unmarshal([]byte(confStr), &v)

				vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
				_, err = root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(v, consts.RunningIntentName, consts.RunningValuesPrio, false), types.NewUpdateInsertFlags(), vpf)
				if err != nil {
					t.Fatalf("failed to import test config: %v", err)
				}

				return root

			},
			importerFunc: func() importer.ImportConfigAdapter {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/1": {
							Name:        ygot.String("ethernet-1/1"),
							Description: ygot.String("my other description"),
						},
					},
				}
				confStr, err := ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
				if err != nil {
					t.Fatalf("failed to marshal test config: %v", err)
				}

				var v any
				json.Unmarshal([]byte(confStr), &v)

				return jsonImporter.NewJsonTreeImporter(v, consts.RunningIntentName, consts.RunningValuesPrio, false)
			},
			resultFunc: func() any {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/1": {
							Name:        ygot.String("ethernet-1/1"),
							Description: ygot.String("my other description"),
						},
					},
				}
				confStr, err := ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
				if err != nil {
					t.Fatalf("failed to marshal test config: %v", err)
				}

				var v any
				json.Unmarshal([]byte(confStr), &v)
				return v
			},
			wantErr: false,
			cacheClientFunc: func(ctrl *gomock.Controller) *mockcacheclient.MockCacheClientBound {
				ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
				ccb.EXPECT().
					IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, excludeIntentNames []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
						close(intentChan)
						close(errChan)
					}).AnyTimes()
				return ccb
			},
			sbiFunc: func(ctrl *gomock.Controller) target.Target {
				sbi := mocktarget.NewMockTarget(ctrl)
				return sbi
			},
		},
	}

	// Run tests
	for _, tt := range tests {

		fmt.Println("----" + tt.name)

		t.Run(tt.name, func(t *testing.T) {
			syncTree := tt.syncTreeFunc()

			ctrl := gomock.NewController(t)

			datastore := &Datastore{
				syncTreeMutex: &sync.RWMutex{},
				dmutex:        &sync.Mutex{},
				syncTree:      syncTree,
				taskPool:      pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)),
				cacheClient:   tt.cacheClientFunc(ctrl),
				sbi:           tt.sbiFunc(ctrl),
			}

			err := datastore.ApplyToRunning(ctx, tt.deletes, tt.importerFunc())
			if (err != nil) != tt.wantErr {
				t.Errorf("ApplyToRunning() error = %v, wantErr %v", err, tt.wantErr)
			}

			ctx := context.Background()

			sc, schema, err := testhelper.InitSDCIOSchema()
			if err != nil {
				t.Fatal(err)
			}
			scb := schemaClient.NewSchemaClientBound(schema, sc)
			tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

			resultRoot, err := tree.NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatalf("failed to create new tree root: %v", err)
			}

			d := tt.resultFunc()

			vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
			_, err = resultRoot.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(d, consts.RunningIntentName, consts.RunningValuesPrio, false), types.NewUpdateInsertFlags(), vpf)
			if err != nil {
				t.Fatalf("failed to import test config: %v", err)
			}

			err = resultRoot.FinishInsertionPhase(ctx)
			if err != nil {
				t.Fatalf("failed to finish insertion phase: %v", err)
			}

			fmt.Println(syncTree.String())

			resetFlagsProcessorParams := &processors.ResetFlagsProcessorParams{NewFlag: true, UpdateFlag: true}
			rpf := processors.NewResetFlagsProcessor(resetFlagsProcessorParams)
			err = rpf.Run(syncTree.Entry, datastore.taskPool)
			if err != nil {
				t.Fatalf("failed to reset flags: %v", err)
			}

			fmt.Println("Adjusted flags count:", rpf.GetAdjustedFlagsCount())

			if diff := cmp.Diff(resultRoot.String(), syncTree.String()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestPerformRevert_HoldsDmutexAcrossSnapshotAndApply verifies that dmutex stays
// held for the full snapshot-diff-apply sequence in performRevert, not just
// around the individual cache read and device write. A concurrent TryLock
// (the pattern TransactionSet/Confirm/Cancel use) must fail for the entire
// duration of the revert, including while the diff is being computed, and
// must succeed again only once performRevert has returned.
func TestPerformRevert_HoldsDmutexAcrossSnapshotAndApply(t *testing.T) {
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatalf("failed to create new tree root: %v", err)
	}

	d := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name:        ygot.String("ethernet-1/1"),
				Description: ygot.String("revert me"),
			},
		},
	}
	confStr, err := ygot.EmitJSON(d, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: false})
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}
	var v any
	json.Unmarshal([]byte(confStr), &v)

	// import as New so it survives into a ToProtoUpdates diff without needing
	// a competing intent from the cache
	flagNew := types.NewUpdateInsertFlags()
	flagNew.SetNewFlag()
	vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	_, err = root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(v, consts.RunningIntentName, consts.RunningValuesPrio, false), flagNew, vpf)
	if err != nil {
		t.Fatalf("failed to import test config: %v", err)
	}
	if err := root.FinishInsertionPhase(ctx); err != nil {
		t.Fatalf("failed to finish insertion phase: %v", err)
	}

	ctrl := gomock.NewController(t)

	ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
	ccb.EXPECT().
		IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, excludeIntentNames []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
			close(intentChan)
			close(errChan)
		}).AnyTimes()

	setStarted := make(chan struct{})
	release := make(chan struct{})
	sbi := mocktarget.NewMockTarget(ctrl)
	sbi.EXPECT().
		Set(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, source any) (*sdcpb.SetDataResponse, error) {
			close(setStarted)
			<-release
			return &sdcpb.SetDataResponse{}, nil
		})

	datastore := &Datastore{
		dmutex:      &sync.Mutex{},
		taskPool:    pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)),
		cacheClient: ccb,
		sbi:         sbi,
	}

	revertDone := make(chan error, 1)
	go func() {
		revertDone <- datastore.performRevert(ctx, root)
	}()

	select {
	case <-setStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for performRevert to reach the device apply")
	}

	// The device write is in flight, so dmutex must still be held: a
	// concurrent TransactionSet-style TryLock must fail.
	if datastore.dmutex.TryLock() {
		datastore.dmutex.Unlock()
		t.Fatal("dmutex.TryLock() succeeded while performRevert's apply was in flight, want held")
	}

	close(release)

	select {
	case err := <-revertDone:
		if err != nil {
			t.Fatalf("performRevert() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for performRevert to return")
	}

	// Now that performRevert has returned, dmutex must be free again.
	if !datastore.dmutex.TryLock() {
		t.Fatal("dmutex.TryLock() failed after performRevert returned, want free")
	}
	datastore.dmutex.Unlock()
}
