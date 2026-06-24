package ops_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	"go.uber.org/mock/gomock"
)

func TestGetSchemaKeysOrders(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, 1))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	doublekeyList, err := tree.NewSharedEntryAttributes(ctx, root.Entry, "doublekey", tc)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("schema declaration order", func(t *testing.T) {
		got := ops.GetSchemaKeys(doublekeyList)
		want := []string{"key2", "key1"}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("GetSchemaKeys() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("alphabetical tree key level order", func(t *testing.T) {
		got := ops.GetSchemaKeysAlphabeticalOrder(doublekeyList)
		want := []string{"key1", "key2"}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("GetSchemaKeysAlphabeticalOrder() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("non-list returns nil", func(t *testing.T) {
		if got := ops.GetSchemaKeysAlphabeticalOrder(root.Entry); got != nil {
			t.Fatalf("GetSchemaKeysAlphabeticalOrder(root) = %v, want nil", got)
		}
	})
}
