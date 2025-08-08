package tree

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/openconfig/ygot/ygot"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

func TestExplicitDeleteVisitor_Visit(t *testing.T) {
	ctx := context.Background()
	owner1 := "owner1"
	owner1Prio := int32(500)
	owner2 := "owner2"
	owner2Prio := int32(50)

	explicitDeleteFlag := types.NewUpdateInsertFlags().SetExplicitDeleteFlag()

	tests := []struct {
		name                 string
		root                 func() *RootEntry
		owner                string
		priority             int32
		explicitDeletes      *sdcpb.PathSet
		wantErr              bool
		expectedLeafVariants LeafVariantSlice
	}{
		{
			name:     "No Deletes",
			owner:    owner2,
			priority: 50,
			root: func() *RootEntry {
				// create a gomock controller
				controller := gomock.NewController(t)
				defer controller.Finish()

				sc, schema, err := testhelper.InitSDCIOSchema()
				if err != nil {
					t.Fatal(err)
				}
				scb := schemaClient.NewSchemaClientBound(schema, sc)
				tc := NewTreeContext(scb, owner1)

				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Error(err)
				}

				jconfStr, err := ygot.EmitJSON(config1(), &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: true,
				})
				if err != nil {
					t.Error(err)
				}

				var jsonConfAny any
				err = json.Unmarshal([]byte(jconfStr), &jsonConfAny)
				if err != nil {
					t.Error(err)
				}

				newFlag := types.NewUpdateInsertFlags()

				err = root.ImportConfig(ctx, types.PathSlice{}, jsonImporter.NewJsonTreeImporter(jsonConfAny), owner1, owner1Prio, newFlag)
				if err != nil {
					t.Error(err)
				}

				return root
			},
		},
		{
			name:     "Delete Field",
			owner:    owner2,
			priority: 50,
			root: func() *RootEntry {
				// create a gomock controller
				controller := gomock.NewController(t)
				defer controller.Finish()

				sc, schema, err := testhelper.InitSDCIOSchema()
				if err != nil {
					t.Fatal(err)
				}
				scb := schemaClient.NewSchemaClientBound(schema, sc)
				tc := NewTreeContext(scb, owner1)

				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Error(err)
				}

				jconfStr, err := ygot.EmitJSON(config1(), &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: true,
				})
				if err != nil {
					t.Error(err)
				}

				var jsonConfAny any
				err = json.Unmarshal([]byte(jconfStr), &jsonConfAny)
				if err != nil {
					t.Error(err)
				}

				newFlag := types.NewUpdateInsertFlags()

				err = root.ImportConfig(ctx, types.PathSlice{}, jsonImporter.NewJsonTreeImporter(jsonConfAny), owner1, owner1Prio, newFlag)
				if err != nil {
					t.Error(err)
				}

				return root
			},
			explicitDeletes: sdcpb.NewPathSet().
				AddPath(
					&sdcpb.Path{
						Elem: []*sdcpb.PathElem{
							sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
							sdcpb.NewPathElem("description", nil),
						},
					},
				),
			expectedLeafVariants: LeafVariantSlice{
				NewLeafEntry(types.NewUpdate(types.PathSlice{"interface", "ethernet-1/1", "description"}, &sdcpb.TypedValue{}, owner2Prio, owner2, 0), explicitDeleteFlag, nil),
			},
		},
		{
			name:     "Delete Branch",
			owner:    owner2,
			priority: 50,
			root: func() *RootEntry {
				// create a gomock controller
				controller := gomock.NewController(t)
				defer controller.Finish()

				sc, schema, err := testhelper.InitSDCIOSchema()
				if err != nil {
					t.Fatal(err)
				}
				scb := schemaClient.NewSchemaClientBound(schema, sc)
				tc := NewTreeContext(scb, owner1)

				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Error(err)
				}

				jconfStr, err := ygot.EmitJSON(config1(), &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: true,
				})
				if err != nil {
					t.Error(err)
				}

				var jsonConfAny any
				err = json.Unmarshal([]byte(jconfStr), &jsonConfAny)
				if err != nil {
					t.Error(err)
				}

				newFlag := types.NewUpdateInsertFlags()

				err = root.ImportConfig(ctx, types.PathSlice{}, jsonImporter.NewJsonTreeImporter(jsonConfAny), owner1, owner1Prio, newFlag)
				if err != nil {
					t.Error(err)
				}

				return root
			},
			explicitDeletes: sdcpb.NewPathSet().
				AddPath(
					&sdcpb.Path{
						Elem: []*sdcpb.PathElem{
							sdcpb.NewPathElem("interface", nil),
						},
					},
				),
			expectedLeafVariants: LeafVariantSlice{
				NewLeafEntry(types.NewUpdate(types.PathSlice{"interface", "ethernet-1/1", "description"}, &sdcpb.TypedValue{}, owner2Prio, owner2, 0), types.NewUpdateInsertFlags().SetExplicitDeleteFlag(), nil),
				NewLeafEntry(types.NewUpdate(types.PathSlice{"interface", "ethernet-1/1", "admin-state"}, &sdcpb.TypedValue{}, owner2Prio, owner2, 0), explicitDeleteFlag, nil),
				NewLeafEntry(types.NewUpdate(types.PathSlice{"interface", "ethernet-1/1", "name"}, &sdcpb.TypedValue{}, owner2Prio, owner2, 0), explicitDeleteFlag, nil),
				NewLeafEntry(types.NewUpdate(types.PathSlice{"interface", "ethernet-1/1", "subinterface", "0", "index"}, &sdcpb.TypedValue{}, owner2Prio, owner2, 0), explicitDeleteFlag, nil),
				NewLeafEntry(types.NewUpdate(types.PathSlice{"interface", "ethernet-1/1", "subinterface", "0", "type"}, &sdcpb.TypedValue{}, owner2Prio, owner2, 0), explicitDeleteFlag, nil),
				NewLeafEntry(types.NewUpdate(types.PathSlice{"interface", "ethernet-1/1", "subinterface", "0", "description"}, &sdcpb.TypedValue{}, owner2Prio, owner2, 0), explicitDeleteFlag, nil),
				NewLeafEntry(types.NewUpdate(types.PathSlice{"interface", "ethernet-1/1", "subinterface", "0", "admin-state"}, &sdcpb.TypedValue{}, owner2Prio, owner2, 0), explicitDeleteFlag, nil),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.root()
			root.AddExplicitDeletes(owner2, tt.priority, tt.explicitDeletes)

			err := root.FinishInsertionPhase(ctx)
			if err != nil {
				t.Error(err)
			}

			lvs := LeafVariantSlice{}
			lvs = root.GetByOwner(owner2, lvs)

			equal, err := lvs.Equal(tt.expectedLeafVariants)
			if err != nil {
				t.Error(err)
			}
			if !equal {
				t.Errorf("result differs from the expected\nExpected:\n%s\nGot:\n%s", tt.expectedLeafVariants.String(), lvs.String())
			}

			t.Log(root.String())
		})
	}
}
