package ops_test

import (
	"context"
	"fmt"
	"runtime"
	"testing"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

func TestContainsOnlyDefaults(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, ctx context.Context, root *tree.RootEntry)
		path  *sdcpb.Path // if nil, test root.Entry; otherwise navigate to this path
		want  bool
	}{
		{
			name: "empty root entry returns true (all children are defaults)",
			setup: func(t *testing.T, ctx context.Context, root *tree.RootEntry) {
				// Empty root entry has no children, so all (zero) children are defaults
			},
			path: nil,
			want: true,
		},
		{
			name: "entry with non-default values returns false",
			setup: func(t *testing.T, ctx context.Context, root *tree.RootEntry) {
				// Add non-default value
				_, err := ops.AddUpdateRecursive(
					ctx,
					root.Entry,
					&sdcpb.Path{
						Elem: []*sdcpb.PathElem{
							sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
							sdcpb.NewPathElem("description", nil),
						},
					},
					types.NewUpdate(nil, testhelper.GetStringTvProto("MyDescription"), 10, "custom-intent", 0),
					types.NewUpdateInsertFlags().SetNewFlag(),
				)
				if err != nil {
					t.Fatal(err)
				}
			},
			path: &sdcpb.Path{
				Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
				},
			},
			want: false,
		},
		{
			name: "entry with child having non-default owner returns false",
			setup: func(t *testing.T, ctx context.Context, root *tree.RootEntry) {
				// Load config with non-default intent on actual data
				_, err := testhelper.LoadYgotStructIntoTreeRoot(
					ctx,
					testhelper.Config1(),
					root.Entry,
					"custom-owner",
					5,
					false,
					testhelper.FlagsNew,
				)
				if err != nil {
					t.Fatal(err)
				}
			},
			path: &sdcpb.Path{
				Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
				},
			},
			want: false,
		},
		{
			name: "entry with multiple children non-defaults returns false",
			setup: func(t *testing.T, ctx context.Context, root *tree.RootEntry) {
				// Add multiple children with non-default owner
				for i := 0; i < 3; i++ {
					_, err := ops.AddUpdateRecursive(
						ctx,
						root.Entry,
						&sdcpb.Path{
							Elem: []*sdcpb.PathElem{
								sdcpb.NewPathElem("interface", map[string]string{"name": fmt.Sprintf("ethernet-1/%d", i+1)}),
								sdcpb.NewPathElem("description", nil),
							},
						},
						types.NewUpdate(nil, testhelper.GetStringTvProto(fmt.Sprintf("Description%d", i)), 10, "custom-intent", 0),
						types.NewUpdateInsertFlags().SetNewFlag(),
					)
					if err != nil {
						t.Fatal(err)
					}
				}
			},
			path: nil,
			want: false,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Common setup for all tests
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
			if err != nil {
				t.Fatal(err)
			}

			tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
			root, err := tree.NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatal(err)
			}

			// Test-specific setup
			tt.setup(t, ctx, root)

			// Finish insertion phase
			if err = root.FinishInsertionPhase(ctx); err != nil {
				t.Fatal(err)
			}

			// Navigate to path or use root
			var entry api.Entry
			if tt.path != nil {
				entry, err = ops.NavigateSdcpbPath(ctx, root.Entry, tt.path)
				if err != nil {
					t.Fatalf("failed to navigate to path %s: %v", tt.path.ToXPath(false), err)
				}
			} else {
				entry = root.Entry
			}

			got := ops.ContainsOnlyDefaults(entry)
			if got != tt.want {
				t.Errorf("ContainsOnlyDefaults() = %v, want %v", got, tt.want)
			}
		})
	}
}
