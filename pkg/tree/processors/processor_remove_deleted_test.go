package processors_test

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"

	"github.com/openconfig/ygot/ygot"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/processors"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// TestRemoveDeletedRootIsNotABranch marks everything under the root as deleted. The root must not be handled
// as a deletable branch of its own: that would return early, remove nothing and leave the entries behind
// once the delete flags are reset. Its entries have to be handled one by one.
func TestRemoveDeletedRootIsNotABranch(t *testing.T) {
	ctx := context.Background()
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	tp := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	root, err := tree.NewTreeRoot(ctx, tree.NewTreeContext(schemaClient.NewSchemaClientBound(schema, sc), tp))
	if err != nil {
		t.Fatal(err)
	}

	str, err := ygot.EmitJSON(&sdcio_schema.Device{Interface: map[string]*sdcio_schema.SdcioModel_Interface{
		"ethernet-1/1": {Name: ygot.String("ethernet-1/1"), Description: ygot.String("a")},
	}}, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: true})
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal([]byte(str), &v); err != nil {
		t.Fatal(err)
	}
	if _, err := root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(v, consts.RunningIntentName, consts.RunningValuesPrio, false), treetypes.NewUpdateInsertFlags(), tp); err != nil {
		t.Fatal(err)
	}
	if err := processors.NewOwnerDeleteMarker(&processors.OwnerDeleteMarkerProcessorParams{Owner: consts.RunningIntentName}).Run(root.Entry, tp); err != nil {
		t.Fatal(err)
	}

	rdp := processors.NewRemoveDeletedProcessor(&processors.RemoveDeletedProcessorParams{Owner: consts.RunningIntentName})
	if err := rdp.Run(root.Entry, tp); err != nil {
		t.Fatal(err)
	}

	// Deletable branches are reported as a whole, their leaves are not visited. Without the root guard the
	// root itself is the only reported branch, which the caller cannot remove.
	emptied := rdp.GetZeroLengthLeafVariantEntries()
	if len(emptied) == 0 {
		t.Fatal("no emptied branch reported, the root was handled as a deletable branch")
	}
	for _, e := range emptied {
		if e.IsRoot() {
			t.Fatal("the root must not be reported as an emptied branch")
		}
	}
}
