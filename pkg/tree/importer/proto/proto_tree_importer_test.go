package proto

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	jimport "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	"go.uber.org/mock/gomock"
)

func TestProtoTreeImporter(t *testing.T) {

	tests := []struct {
		name  string
		input string
	}{
		{
			name: "One",
			input: `
				{
			  "choices": {
    "case1": {
      "case-elem": {
        "elem": "foocaseval"
      }
    }
  },
			"interface": [
				{
				"admin-state": "enable",
				"description": "Foo",
				"name": "ethernet-1/2",
				"subinterface": [
					{
					"description": "Subinterface 5",
					"index": 5,
					"type": "routed"
					}
				]
				},
								{
				"admin-state": "enable",
				"description": "Bar",
				"name": "ethernet-1/3",
				"subinterface": [
					{
					"description": "Subinterface 5",
					"index": 5,
					"type": "routed"
					}
				]
				}
				,				{
				"admin-state": "enable",
				"description": "FooBar",
				"name": "ethernet-1/4",
				"subinterface": [
					{
					"description": "Subinterface 5",
					"index": 5,
					"type": "routed"
					},					{
					"description": "Subinterface 6",
					"index": 6,
					"type": "routed"
					},					{
					"description": "Subinterface 7",
					"index": 7,
					"type": "routed"
					}
				]
				}
			],
			"leaflist": {
				"entry": [
				"foo",
				"bar"
				]
			},
			"network-instance": [
				{
				"admin-state": "enable",
				"description": "Other NI",
				"name": "other",
				"type": "ip-vrf",
				"protocol":{
					"bgp": {}
				}
				}
			],
			"patterntest": "hallo DU",
			"emptyconf": {}
			}`,
		},
	}

	// create a gomock controller
	controller := gomock.NewController(t)

	scb, err := testhelper.GetSchemaClientBound(t, controller)
	if err != nil {
		t.Error(err)
	}
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
			root, err := tree.NewTreeRoot(ctx, tc)
			if err != nil {
				t.Error(err)
			}

			jsonBytes := []byte(tt.input)

			var j any
			err = json.Unmarshal(jsonBytes, &j)
			if err != nil {
				t.Fatalf("error parsing json document: %v", err)
			}

			jti := jimport.NewJsonTreeImporter(j, "owner1", 5, false)

			vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
			_, err = root.ImportConfig(ctx, nil, jti, types.NewUpdateInsertFlags(), vpf)
			if err != nil {
				t.Fatal(err)
			}

			err = root.FinishInsertionPhase(ctx)
			if err != nil {
				t.Error(err)
			}
			t.Log(root.String())

			protoIntent, err := root.TreeExport("owner1", 5)
			if err != nil {
				t.Error(err)
			}

			fmt.Println(protoIntent.PrettyString("  "))

			tcNew := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
			rootNew, err := tree.NewTreeRoot(ctx, tcNew)
			if err != nil {
				t.Error(err)
			}

			protoAdapter := NewProtoTreeImporter(protoIntent)

			vpf2 := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
			_, err = rootNew.ImportConfig(ctx, nil, protoAdapter, types.NewUpdateInsertFlags(), vpf2)
			if err != nil {
				t.Error(err)
			}
			err = rootNew.FinishInsertionPhase(ctx)
			if err != nil {
				t.Error(err)
			}
			t.Log(rootNew.String())

			if diff := cmp.Diff(root.String(), rootNew.String()); diff != "" {
				t.Errorf("Error imported data differs:%s", diff)
			}
		})
	}
}
