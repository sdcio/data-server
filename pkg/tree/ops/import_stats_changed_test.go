package ops_test

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
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestImportStatsChanged(t *testing.T) {
	ctx := context.Background()
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)
	tp := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	tc := tree.NewTreeContext(scb, tp)
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	conf := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name:        ygot.String("ethernet-1/1"),
				Description: ygot.String("same"),
			},
		},
	}
	confStr, err := ygot.EmitJSON(conf, &ygot.EmitJSONConfig{Format: ygot.RFC7951})
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal([]byte(confStr), &v); err != nil {
		t.Fatal(err)
	}

	importRunning := func(device any) *treetypes.ImportStats {
		t.Helper()
		stats, err := root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(device, consts.RunningIntentName, consts.RunningValuesPrio, false), treetypes.NewUpdateInsertFlags(), tp)
		if err != nil {
			t.Fatal(err)
		}
		return stats
	}

	firstStats := importRunning(v)
	if !firstStats.Changed() {
		t.Fatal("first import stats should report changed")
	}

	secondStats := importRunning(v)
	if secondStats.Changed() {
		t.Fatal("identical re-import should not report changed")
	}

	conf.Interface["ethernet-1/1"].Description = ygot.String("updated")
	updatedStr, err := ygot.EmitJSON(conf, &ygot.EmitJSONConfig{Format: ygot.RFC7951})
	if err != nil {
		t.Fatal(err)
	}
	var updated any
	if err := json.Unmarshal([]byte(updatedStr), &updated); err != nil {
		t.Fatal(err)
	}
	updatedStats := importRunning(updated)
	if !updatedStats.Changed() {
		t.Fatal("updated leaf import stats should report changed")
	}
}
