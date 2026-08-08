package validation_test

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/ops/validation"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// addServerLeaf adds a single leaf value for a server list instance.
func addServerLeaf(ctx context.Context, t *testing.T, root *tree.RootEntry, name, leaf string, val *sdcpb.TypedValue, owner string, prio int32, flags *types.UpdateInsertFlags) {
	t.Helper()
	p := &sdcpb.Path{
		Elem: []*sdcpb.PathElem{
			sdcpb.NewPathElem("list-unique", nil),
			sdcpb.NewPathElem("server", map[string]string{"name": name}),
			sdcpb.NewPathElem(leaf, nil),
		},
	}
	_, err := ops.AddUpdateRecursive(ctx, root.Entry, p, types.NewUpdate(nil, val, prio, owner, 0), flags)
	if err != nil {
		t.Fatalf("addServerLeaf(%s/%s): %v", name, leaf, err)
	}
}

// addMultiSegLeaf adds a leaf value to a multi-seg list instance.
func addMultiSegLeaf(ctx context.Context, t *testing.T, root *tree.RootEntry, name string, val *sdcpb.TypedValue, owner string, prio int32) {
	t.Helper()
	p := &sdcpb.Path{
		Elem: []*sdcpb.PathElem{
			sdcpb.NewPathElem("list-unique", nil),
			sdcpb.NewPathElem("multi-seg", map[string]string{"name": name}),
			sdcpb.NewPathElem("sub", nil),
			sdcpb.NewPathElem("field", nil),
		},
	}
	flags := types.NewUpdateInsertFlags().SetNewFlag()
	_, err := ops.AddUpdateRecursive(ctx, root.Entry, p, types.NewUpdate(nil, val, prio, owner, 0), flags)
	if err != nil {
		t.Fatalf("addMultiSegLeaf(%s): %v", name, err)
	}
}

var serverListPath = []*sdcpb.PathElem{
	sdcpb.NewPathElem("list-unique", nil),
	sdcpb.NewPathElem("server", nil),
}

var multiSegListPath = []*sdcpb.PathElem{
	sdcpb.NewPathElem("list-unique", nil),
	sdcpb.NewPathElem("multi-seg", nil),
}

func TestValidateUnique(t *testing.T) {
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	newFlags := types.NewUpdateInsertFlags().SetNewFlag()

	tests := []struct {
		name                string
		setup               func(t *testing.T, root *tree.RootEntry)
		listPath            []*sdcpb.PathElem
		wantErrors          int
		wantWarnings        bool
		wantUniqueStatCount uint32
	}{
		{
			name: "valid config - distinct tuples",
			setup: func(t *testing.T, root *tree.RootEntry) {
				addServerLeaf(ctx, t, root, "srv1", "ip", testhelper.GetStringTvProto("1.2.3.4"), "owner1", 100, newFlags)
				addServerLeaf(ctx, t, root, "srv1", "port", testhelper.GetUIntTvProto(8080), "owner1", 100, newFlags)
				addServerLeaf(ctx, t, root, "srv2", "ip", testhelper.GetStringTvProto("1.2.3.5"), "owner1", 100, newFlags)
				addServerLeaf(ctx, t, root, "srv2", "port", testhelper.GetUIntTvProto(8080), "owner1", 100, newFlags)
			},
			listPath:            serverListPath,
			wantErrors:          0,
			wantUniqueStatCount: 2,
		},
		{
			name: "collision - two entries sharing (ip, port)",
			setup: func(t *testing.T, root *tree.RootEntry) {
				addServerLeaf(ctx, t, root, "srv1", "ip", testhelper.GetStringTvProto("1.2.3.4"), "owner1", 100, newFlags)
				addServerLeaf(ctx, t, root, "srv1", "port", testhelper.GetUIntTvProto(8080), "owner1", 100, newFlags)
				addServerLeaf(ctx, t, root, "srv2", "ip", testhelper.GetStringTvProto("1.2.3.4"), "owner1", 100, newFlags)
				addServerLeaf(ctx, t, root, "srv2", "port", testhelper.GetUIntTvProto(8080), "owner1", 100, newFlags)
			},
			listPath:            serverListPath,
			wantErrors:          2,
			wantUniqueStatCount: 2,
		},
		{
			name: "deleted entry excluded from check",
			setup: func(t *testing.T, root *tree.RootEntry) {
				// srv1 is being explicitly deleted — must not participate in the uniqueness check
				delFlags := types.NewUpdateInsertFlags().SetExplicitDeleteFlag()
				addServerLeaf(ctx, t, root, "srv1", "ip", testhelper.GetStringTvProto("1.2.3.4"), "owner1", 100, delFlags)
				addServerLeaf(ctx, t, root, "srv1", "port", testhelper.GetUIntTvProto(8080), "owner1", 100, delFlags)
				// srv2 carries the same values — no collision because srv1 is being deleted
				addServerLeaf(ctx, t, root, "srv2", "ip", testhelper.GetStringTvProto("1.2.3.4"), "owner1", 100, newFlags)
				addServerLeaf(ctx, t, root, "srv2", "port", testhelper.GetUIntTvProto(8080), "owner1", 100, newFlags)
			},
			listPath:            serverListPath,
			wantErrors:          0,
			wantUniqueStatCount: 2,
		},
		{
			name: "missing constrained leaf excluded - RFC 7950 §7.8.3",
			setup: func(t *testing.T, root *tree.RootEntry) {
				// neither entry has port — both are excluded from the (ip, port) constraint
				addServerLeaf(ctx, t, root, "srv1", "ip", testhelper.GetStringTvProto("1.2.3.4"), "owner1", 100, newFlags)
				addServerLeaf(ctx, t, root, "srv2", "ip", testhelper.GetStringTvProto("1.2.3.4"), "owner1", 100, newFlags)
			},
			listPath:            serverListPath,
			wantErrors:          0,
			wantUniqueStatCount: 2,
		},
		{
			name: "multi-segment element - constraint skipped with warning",
			setup: func(t *testing.T, root *tree.RootEntry) {
				// both entries share the same sub/field value; would collide if enforced
				addMultiSegLeaf(ctx, t, root, "ms1", testhelper.GetStringTvProto("same"), "owner1", 100)
				addMultiSegLeaf(ctx, t, root, "ms2", testhelper.GetStringTvProto("same"), "owner1", 100)
			},
			listPath:            multiSegListPath,
			wantErrors:          0,
			wantWarnings:        true,
			wantUniqueStatCount: 1,
		},
		{
			name: "two independent unique statements enforced separately",
			setup: func(t *testing.T, root *tree.RootEntry) {
				// unique "ip port": srv1 and srv2 differ → no violation
				// unique "description": both share "dup-desc" → 2 errors (one per colliding entry)
				addServerLeaf(ctx, t, root, "srv1", "ip", testhelper.GetStringTvProto("1.2.3.4"), "owner1", 100, newFlags)
				addServerLeaf(ctx, t, root, "srv1", "port", testhelper.GetUIntTvProto(8080), "owner1", 100, newFlags)
				addServerLeaf(ctx, t, root, "srv1", "description", testhelper.GetStringTvProto("dup-desc"), "owner1", 100, newFlags)
				addServerLeaf(ctx, t, root, "srv2", "ip", testhelper.GetStringTvProto("1.2.3.5"), "owner1", 100, newFlags)
				addServerLeaf(ctx, t, root, "srv2", "port", testhelper.GetUIntTvProto(9090), "owner1", 100, newFlags)
				addServerLeaf(ctx, t, root, "srv2", "description", testhelper.GetStringTvProto("dup-desc"), "owner1", 100, newFlags)
			},
			listPath:            serverListPath,
			wantErrors:          2,
			wantUniqueStatCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

			root, err := tree.NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatal(err)
			}

			tt.setup(t, root)

			if err := root.FinishInsertionPhase(ctx); err != nil {
				t.Fatalf("FinishInsertionPhase: %v", err)
			}

			e, err := ops.NavigateSdcpbPath(ctx, root.Entry, &sdcpb.Path{Elem: tt.listPath})
			if err != nil {
				t.Fatalf("navigate: %v", err)
			}

			vCfg := config.NewValidationConfig()
			vCfg.DisabledValidators.DisableAll()
			vCfg.DisabledValidators.Unique = false
			vCfg.DisableConcurrency = true

			result, stats := validation.Validate(ctx, e, vCfg, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

			t.Logf("Validation Errors:\n%s", strings.Join(result.ErrorsStr(), "\n"))
			t.Log(root.String())

			if got := len(result.ErrorsStr()); got != tt.wantErrors {
				t.Errorf("expected %d error(s), got %d: %v", tt.wantErrors, got, result)
			}
			if tt.wantWarnings && !result.HasWarnings() {
				t.Error("expected at least one warning, got none")
			}
			if got := stats.GetCounter()[types.StatTypeUnique]; got != tt.wantUniqueStatCount {
				t.Errorf("expected unique stat count %d, got %d", tt.wantUniqueStatCount, got)
			}
		})
	}
}
