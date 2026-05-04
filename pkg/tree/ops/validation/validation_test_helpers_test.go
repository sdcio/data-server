package validation
package validation_test

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"

	"github.com/openconfig/ygot/ygot"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	json_importer "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
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

	configJSON, err := ygot.EmitJSON(req, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: true})
	if err != nil {
		t.Fatal(err)
	}

	var jsonConfig any
	if err = json.Unmarshal([]byte(configJSON), &jsonConfig); err != nil {
		t.Fatal(err)
	}

	jimporter := json_importer.NewJsonTreeImporter(jsonConfig, "owner1", 5, false)
	sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

	if _, err = root.ImportConfig(ctx, &sdcpb.Path{}, jimporter, types.NewUpdateInsertFlags(), sharedPool); err != nil {
		t.Fatal(err)
	}

	if err = root.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}

	return root, sharedPool
}
