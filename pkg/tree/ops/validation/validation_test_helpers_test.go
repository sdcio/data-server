package validation_test

import (
	"context"
	"runtime"
	"testing"

	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
)

// importDeviceJSON creates a fresh tree, imports req via JSON (owner "owner1", priority 5),
// finishes the insertion phase, and returns the root entry and a shared pool ready for validation.
func importDeviceJSON(t *testing.T, ctx context.Context, scb schemaClient.SchemaClientBound, req *sdcio_schema.Device) (*tree.RootEntry, *pool.SharedTaskPool) {
	t.Helper()

	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, req, root.Entry, "owner1", 5, false, types.NewUpdateInsertFlags()); err != nil {
		t.Fatal(err)
	}

	if err = root.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}

	sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	return root, sharedPool
}
