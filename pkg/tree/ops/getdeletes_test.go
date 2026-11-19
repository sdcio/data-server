package ops_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/processors"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

func TestGetDeletes_MultiKeyListEntryDeletion(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, ctx context.Context, root *tree.RootEntry, p pool.VirtualPoolFactory)
		want  []string
	}{
		{
			name: "sole multi-key entry deleted with unmanaged child in running emits full key path",
			setup: func(t *testing.T, ctx context.Context, root *tree.RootEntry, p pool.VirtualPoolFactory) {
				intentJSON := map[string]any{
					"doublekey": []any{
						map[string]any{
							"key2":    "k2foo",
							"key1":    "k1bar",
							"mandato": "val1",
						},
					},
				}
				runningJSON := map[string]any{
					"doublekey": []any{
						map[string]any{
							"key2":    "k2foo",
							"key1":    "k1bar",
							"mandato": "val1",
							"cont": map[string]any{
								"value1": "unmanaged",
							},
						},
					},
				}
				if _, err := root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(intentJSON, "intent1", 10, false), testhelper.FlagsExisting, p); err != nil {
					t.Fatal(err)
				}
				if _, err := root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(runningJSON, consts.RunningIntentName, consts.RunningValuesPrio, false), testhelper.FlagsExisting, p); err != nil {
					t.Fatal(err)
				}
				if err := processors.NewOwnerDeleteMarker(&processors.OwnerDeleteMarkerProcessorParams{Owner: "intent1", OnlyIntended: false}).Run(root.Entry, p); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"/doublekey[key1=k1bar][key2=k2foo]"},
		},
		{
			name: "explicit delete of single key leaf promotes to list entry delete without sibling leaf deletes",
			setup: func(t *testing.T, ctx context.Context, root *tree.RootEntry, p pool.VirtualPoolFactory) {
				runningJSON := map[string]any{
					"doublekey": []any{
						map[string]any{
							"key2":    "k2foo",
							"key1":    "k1bar",
							"mandato": "val1",
						},
						map[string]any{
							"key2":    "k2other",
							"key1":    "k1bar",
							"mandato": "val2",
						},
					},
				}
				if _, err := root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(runningJSON, consts.RunningIntentName, consts.RunningValuesPrio, false), testhelper.FlagsExisting, p); err != nil {
					t.Fatal(err)
				}
				ps := sdcpb.NewPathSet().
					AddPath(&sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "doublekey", Key: map[string]string{"key2": "k2foo", "key1": "k1bar"}}, {Name: "key2"}}}).
					AddPath(&sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "doublekey", Key: map[string]string{"key2": "k2foo", "key1": "k1bar"}}, {Name: "mandato"}}})
				root.GetTreeContext().ExplicitDeletes().Add(consts.RevertRunningName, consts.RevertRunningPrio, ps)
			},
			want: []string{"/doublekey[key1=k1bar][key2=k2foo]"},
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
			if err != nil {
				t.Fatal(err)
			}

			p := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
			tc := tree.NewTreeContext(scb, p)
			root, err := tree.NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatal(err)
			}

			tt.setup(t, ctx, root, p)

			if err := root.FinishInsertionPhase(ctx); err != nil {
				t.Fatal(err)
			}

			dels, err := ops.GetDeletes(root.Entry, true)
			if err != nil {
				t.Fatal(err)
			}

			got := dels.SdcpbPaths().ToXPathSlice()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("GetDeletes() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
