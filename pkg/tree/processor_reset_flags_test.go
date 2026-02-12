package tree

import (
	"context"
	"fmt"
	"runtime"
	"testing"

	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResetFlagsProcessorRun tests the Run method of ResetFlagsProcessor
func TestResetFlagsProcessorRun(t *testing.T) {
	tests := []struct {
		name        string
		deleteFlag  bool
		newFlag     bool
		updateFlag  bool
		wantErr     bool
		tree        func() *RootEntry
		adjustCount int64
	}{
		{
			name:        "successful reset with all flags",
			deleteFlag:  true,
			newFlag:     true,
			updateFlag:  true,
			wantErr:     false,
			adjustCount: 4,
			tree: func() *RootEntry {

				ctx := context.Background()

				sc, schema, err := testhelper.InitSDCIOSchema()
				if err != nil {
					t.Fatal(err)
				}
				scb := schemaClient.NewSchemaClientBound(schema, sc)
				tc := NewTreeContext(scb,  pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatalf("failed to create new tree root: %v", err)
				}

				_, err = root.AddUpdateRecursive(ctx,
					&sdcpb.Path{Elem: []*sdcpb.PathElem{
						sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
						sdcpb.NewPathElem("description", nil)},
					},
					types.NewUpdate(nil, testhelper.GetStringTvProto("desc 1/1"), RunningValuesPrio, RunningIntentName, 0),
					flagsNew,
				)
				if err != nil {
					t.Fatalf("failed to add update recursive: %v", err)
				}

				_, err = root.AddUpdateRecursive(ctx,
					&sdcpb.Path{Elem: []*sdcpb.PathElem{
						sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
						sdcpb.NewPathElem("description", nil)},
					},
					types.NewUpdate(nil, testhelper.GetStringTvProto("desc 1/2"), RunningValuesPrio, RunningIntentName, 0),
					types.NewUpdateInsertFlags().SetDeleteFlag(),
				)
				if err != nil {
					t.Fatalf("failed to add update recursive: %v", err)
				}

				err = root.FinishInsertionPhase(ctx)
				if err != nil {
					t.Fatalf("failed to finish insertion phase: %v", err)
				}

				return root

			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create processor parameters
			params := NewResetFlagsProcessorParameters()

			if tt.deleteFlag {
				params.SetDeleteFlag()
			}
			if tt.newFlag {
				params.SetNewFlag()
			}
			if tt.updateFlag {
				params.SetUpdateFlag()
			}

			// Create processor
			processor := NewResetFlagsProcessor(params)
			require.NotNil(t, processor)

			// Create a mock entry for testing
			// Note: In a real test, you would use a properly initialized Entry
			// This is a simplified version - you may need to adjust based on your actual Entry implementation

			// Create a virtual pool for testing
			taskPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

			root := tt.tree()

			fmt.Println(root.String())

			processorErr := processor.Run(root.GetRoot(), taskPool)
			if (processorErr != nil) != tt.wantErr {
				t.Errorf("ResetFlagsProcessor.Run() error = %v, wantErr %v", processorErr, tt.wantErr)
				return
			}

			fmt.Println(root.String())

			// For now, we'll test that the processor can be created and run method exists
			// Full integration testing would require properly mocked Entry objects
			assert.NotNil(t, processor)
			assert.Equal(t, int64(tt.adjustCount), params.GetAdjustedFlagsCount())
		})
	}
}
