package datastore

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/mocks/mockcacheclient"
	"github.com/sdcio/data-server/mocks/mocktarget"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/datastore/target"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/importer"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
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
				tc := tree.NewTreeContext(scb, tree.RunningIntentName)

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

				vpf := pool.NewSharedTaskPool(ctx, runtime.NumCPU())
				err = root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(v, tree.RunningIntentName, tree.RunningValuesPrio, false), types.NewUpdateInsertFlags(), vpf)
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

				return jsonImporter.NewJsonTreeImporter(v, tree.RunningIntentName, tree.RunningValuesPrio, false)
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
				tc := tree.NewTreeContext(scb, tree.RunningIntentName)

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

				vpf := pool.NewSharedTaskPool(ctx, runtime.NumCPU())
				err = root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(v, tree.RunningIntentName, tree.RunningValuesPrio, false), types.NewUpdateInsertFlags(), vpf)
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

				return jsonImporter.NewJsonTreeImporter(v, tree.RunningIntentName, tree.RunningValuesPrio, false)
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
				tc := tree.NewTreeContext(scb, tree.RunningIntentName)

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

				vpf := pool.NewSharedTaskPool(ctx, runtime.NumCPU())
				err = root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(v, tree.RunningIntentName, tree.RunningValuesPrio, false), types.NewUpdateInsertFlags(), vpf)
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

				return jsonImporter.NewJsonTreeImporter(v, tree.RunningIntentName, tree.RunningValuesPrio, false)
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
				syncTree:      syncTree,
				taskPool:      pool.NewSharedTaskPool(ctx, runtime.NumCPU()),
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
			tc := tree.NewTreeContext(scb, tree.RunningIntentName)

			resultRoot, err := tree.NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatalf("failed to create new tree root: %v", err)
			}

			d := tt.resultFunc()

			vpf := pool.NewSharedTaskPool(ctx, runtime.NumCPU())
			err = resultRoot.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(d, tree.RunningIntentName, tree.RunningValuesPrio, false), types.NewUpdateInsertFlags(), vpf)
			if err != nil {
				t.Fatalf("failed to import test config: %v", err)
			}

			err = resultRoot.FinishInsertionPhase(ctx)
			if err != nil {
				t.Fatalf("failed to finish insertion phase: %v", err)
			}

			fmt.Println(syncTree.String())

			resetFlagsProcessorParams := tree.NewResetFlagsProcessorParameters(false, true, true)
			err = tree.NewResetFlagsProcessor(resetFlagsProcessorParams).Run(syncTree.GetRoot(), datastore.taskPool)
			if err != nil {
				t.Fatalf("failed to reset flags: %v", err)
			}

			fmt.Println("Adjusted flags count:", resetFlagsProcessorParams.GetAdjustedFlagsCount())

			if diff := cmp.Diff(resultRoot.String(), syncTree.String()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
