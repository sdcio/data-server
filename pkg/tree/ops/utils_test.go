package ops_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// TestToJson_ListEntryKeyWithoutLeafVariant reproduces a panic scenario where a
// list entry's key leaf exists in the tree (e.g. as a structural/placeholder
// entry created while navigating the tree) but never got a LeafVariant assigned
// to it and has no active children either. Sorting such list entries for JSON
// rendering used to index into an empty slice returned by GetHighestPrecedence,
// causing an "index out of range [0] with length 0" panic. ToJson must instead
// treat such incomparable entries as equal and render without panicking.
func TestToJson_ListEntryKeyWithoutLeafVariant(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	// Create two list entries purely via structural navigation (no updates / no
	// LeafVariants applied), leaving the "name" key entries valueless, similar to
	// how a placeholder path can end up in the tree without ever carrying data.
	for _, name := range []string{"ethernet-1/1", "ethernet-1/2"} {
		keyNode, err := ops.GetOrCreateChilds(ctx, root.Entry, &sdcpb.Path{
			Elem: []*sdcpb.PathElem{
				sdcpb.NewPathElem("interface", map[string]string{"name": name}),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		// Attach the "name" key leaf entry structurally, without ever adding a
		// LeafVariant to it.
		if _, err := api.NewEntry(ctx, keyNode, "name", tc); err != nil {
			t.Fatal(err)
		}
	}

	if err := root.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}

	// Must not panic.
	if _, err := ops.ToJson(ctx, root.Entry, false); err != nil {
		t.Fatalf("ToJson() returned unexpected error: %v", err)
	}
}
