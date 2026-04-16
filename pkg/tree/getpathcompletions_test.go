package tree

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"

	"github.com/openconfig/ygot/ygot"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/consts"
	json_importer "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
)

func TestGetPathCompletions_Integration(t *testing.T) {
	testCases := []struct {
		name       string
		toComplete string
		expects    []string
	}{
		{
			name:       "interface root completion",
			toComplete: "/int",
			expects:    []string{"/interface", "/interface[name="},
		},
		{
			name:       "interface key name completion",
			toComplete: "/interface[",
			expects:    []string{"/interface[name="},
		},
		{
			name:       "interface key value completion",
			toComplete: "/interface[name=eth",
			expects:    []string{"/interface[name=ethernet-1/1]"},
		},
		{
			name:       "doublekey root completion",
			toComplete: "/doublekey",
			expects:    []string{"/doublekey[key1=k1ey1]", "/doublekey[key1=k1ey2]"},
		},
		{
			name:       "doublekey key1 completion",
			toComplete: "/doublekey[key1=",
			expects:    []string{"/doublekey[key1=k1ey1]", "/doublekey[key1=k1ey2]"},
		},
		{
			name:       "doublekey key2 completion",
			toComplete: "/doublekey[key1=k1ey1]",
			expects:    []string{"/doublekey[key1=k1ey1][key2="},
		},
		{
			name:       "doublekey key2 partial completion",
			toComplete: "/doublekey[key1=k1ey1][key2=k2",
			expects:    []string{"/doublekey[key1=k1ey1][key2=k2ey2]", "/doublekey[key1=k1ey1][key2=k2ey3]"},
		},
		{
			name:       "doublekey key2 first partial completion",
			toComplete: "/doublekey[key2=k2ey",
			expects:    []string{"/doublekey[key2=k2ey3]", "/doublekey[key2=k2ey2]"},
		},
		{
			name:       "doublekey key2 first",
			toComplete: "/doublekey[key2=k2ey3]",
			expects:    []string{"/doublekey[key2=k2ey3][key1="},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sc, schema, err := testhelper.InitSDCIOSchema()
			if err != nil {
				t.Fatalf("InitSDCIOSchema failed: %v", err)
			}
			scb := schemaClient.NewSchemaClientBound(schema, sc)
			ctx := context.Background()

			treeContext := NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
			root, err := NewTreeRoot(ctx, treeContext)
			if err != nil {
				t.Fatalf("NewTreeRoot failed: %v", err)
			}

			flagsNew := types.NewUpdateInsertFlags()
			flagsNew.SetNewFlag()

			d := testhelper.Config1()
			d.Doublekey = map[sdcio_schema.SdcioModel_Doublekey_Key]*sdcio_schema.SdcioModel_Doublekey{
				{Key1: "k1ey1", Key2: "k2ey2"}: {
					Key1: ygot.String("k1ey1"),
					Key2: ygot.String("k2ey2"),
				},
				{Key1: "k1ey1", Key2: "k2ey3"}: {
					Key1: ygot.String("k1ey1"),
					Key2: ygot.String("k2ey3"),
				},
				{Key1: "k1ey2", Key2: "k2ey3"}: {
					Key1: ygot.String("k1ey2"),
					Key2: ygot.String("k2ey3"),
				},
			}
			jconfStr, err := ygot.EmitJSON(d, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: false})
			if err != nil {
				t.Fatalf("ygot.EmitJSON failed: %v", err)
			}

			var jsonConf any

			err = json.Unmarshal([]byte(jconfStr), &jsonConf)
			if err != nil {
				t.Error(err)
			}

			importer := json_importer.NewJsonTreeImporter(jsonConf, consts.RunningIntentName, consts.RunningValuesPrio, false)
			vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
			_, err = root.ImportConfig(ctx, nil, importer, flagsNew, vpf)
			if err != nil {
				t.Fatalf("ImportConfig failed: %v", err)
			}

			err = root.FinishInsertionPhase(ctx)
			if err != nil {
				t.Fatalf("FinishInsertionPhase failed: %v", err)
			}

			results := ops.GetPathCompletions(ctx, root.Entry, tc.toComplete)
			for _, expect := range tc.expects {
				found := false
				for _, got := range results {
					if got == expect {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected completion %q not found in results: %v", expect, results)
				}
			}
		})
	}
}
