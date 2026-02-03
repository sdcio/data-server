package tree

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/openconfig/ygot/ygot"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
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
				tc := NewTreeContext(scb, owner1, pool.NewSharedTaskPool(ctx, runtime.NumCPU()))

				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Error(err)
				}

				_, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, config1(), root, owner1, owner1Prio, false, flagsNew)
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
				tc := NewTreeContext(scb, owner1, pool.NewSharedTaskPool(ctx, runtime.NumCPU()))

				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Error(err)
				}

				_, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, config1(), root, owner1, owner1Prio, false, flagsExisting)
				if err != nil {
					t.Error(err)
				}

				_, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, config1(), root, RunningIntentName, RunningValuesPrio, false, flagsExisting)
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
				NewLeafEntry(types.NewUpdate(testhelper.NewUpdateParentMock(&sdcpb.Path{
					IsRootBased: true,
					Elem: []*sdcpb.PathElem{
						sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
						sdcpb.NewPathElem("description", nil),
					},
				}), &sdcpb.TypedValue{}, owner2Prio, owner2, 0), explicitDeleteFlag, nil),
			},
		},
		{
			name:     "Delete Branch - existing owner data",
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
				tc := NewTreeContext(scb, owner1, pool.NewSharedTaskPool(ctx, runtime.NumCPU()))

				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Error(err)
				}

				_, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, config1(), root, owner1, owner1Prio, false, flagsExisting)
				if err != nil {
					t.Error(err)
				}

				_, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, config1(), root, RunningIntentName, RunningValuesPrio, false, flagsExisting)
				if err != nil {
					t.Error(err)
				}

				testhelper.LoadYgotStructIntoTreeRoot(ctx, &sdcio_schema.Device{Interface: map[string]*sdcio_schema.SdcioModel_Interface{
					"ethernet-1/1": {
						Name:        ygot.String("ethernet-1/1"),
						Description: ygot.String("mydesc"),
					},
				}}, root, owner2, owner2Prio, false, flagsNew)

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
				NewLeafEntry(
					types.NewUpdate(
						testhelper.NewUpdateParentMock(&sdcpb.Path{
							IsRootBased: true,
							Elem: []*sdcpb.PathElem{
								sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
								sdcpb.NewPathElem("description", nil),
							},
						}),
						&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "mydesc"}},
						owner2Prio, owner2, 0,
					),
					types.NewUpdateInsertFlags().SetExplicitDeleteFlag(),
					nil,
				),
				NewLeafEntry(
					types.NewUpdate(
						testhelper.NewUpdateParentMock(&sdcpb.Path{
							IsRootBased: true,
							Elem: []*sdcpb.PathElem{
								sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
								sdcpb.NewPathElem("admin-state", nil),
							},
						}),
						&sdcpb.TypedValue{},
						owner2Prio, owner2, 0,
					),
					explicitDeleteFlag,
					nil,
				),
				NewLeafEntry(
					types.NewUpdate(
						testhelper.NewUpdateParentMock(&sdcpb.Path{
							IsRootBased: true,
							Elem: []*sdcpb.PathElem{
								sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
								sdcpb.NewPathElem("name", nil),
							},
						}),
						&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "ethernet-1/1"}},
						owner2Prio, owner2, 0,
					),
					explicitDeleteFlag,
					nil,
				),
				NewLeafEntry(
					types.NewUpdate(
						testhelper.NewUpdateParentMock(&sdcpb.Path{
							IsRootBased: true,
							Elem: []*sdcpb.PathElem{
								sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
								sdcpb.NewPathElem("subinterface", map[string]string{"index": "0"}),
								sdcpb.NewPathElem("index", nil),
							},
						}),
						&sdcpb.TypedValue{},
						owner2Prio, owner2, 0,
					),
					explicitDeleteFlag,
					nil,
				),
				NewLeafEntry(
					types.NewUpdate(
						testhelper.NewUpdateParentMock(&sdcpb.Path{
							IsRootBased: true,
							Elem: []*sdcpb.PathElem{
								sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
								sdcpb.NewPathElem("subinterface", map[string]string{"index": "0"}),
								sdcpb.NewPathElem("type", nil),
							},
						}),
						&sdcpb.TypedValue{},
						owner2Prio, owner2, 0,
					),
					explicitDeleteFlag,
					nil,
				),
				NewLeafEntry(
					types.NewUpdate(
						testhelper.NewUpdateParentMock(&sdcpb.Path{
							IsRootBased: true,
							Elem: []*sdcpb.PathElem{
								sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
								sdcpb.NewPathElem("subinterface", map[string]string{"index": "0"}),
								sdcpb.NewPathElem("description", nil),
							},
						}),
						&sdcpb.TypedValue{},
						owner2Prio, owner2, 0,
					),
					explicitDeleteFlag,
					nil,
				),
				NewLeafEntry(
					types.NewUpdate(
						testhelper.NewUpdateParentMock(&sdcpb.Path{
							IsRootBased: true,
							Elem: []*sdcpb.PathElem{
								sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
								sdcpb.NewPathElem("subinterface", map[string]string{"index": "0"}),
								sdcpb.NewPathElem("admin-state", nil),
							},
						}),
						&sdcpb.TypedValue{},
						owner2Prio, owner2, 0,
					),
					explicitDeleteFlag,
					nil,
				),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.root()
			root.treeContext.AddExplicitDeletes(owner2, tt.priority, tt.explicitDeletes)

			err := root.FinishInsertionPhase(ctx)
			if err != nil {
				t.Error(err)
			}

			t.Log(root.String())

			lvs := LeafVariantSlice{}
			lvs = root.GetByOwner(owner2, lvs)

			equal, err := lvs.Equal(tt.expectedLeafVariants)
			if err != nil {
				t.Error(err)
			}
			if !equal {
				if diff := testhelper.SplitStringSortDiff(tt.expectedLeafVariants.String(), lvs.String(), "\n"); diff != "" {
					t.Errorf("mismatch (-want +got)\n%s", diff)
					return
				}
			}

			// get the resulting deletes
			dels, err := root.GetDeletes(true)
			if err != nil {
				t.Error(err)
				return
			}
			t.Logf("Deletes:\n%s", strings.Join(dels.SdcpbPaths().ToXPathSlice(), "\n"))
			updates := root.GetHighestPrecedence(true)

			t.Logf("Updates:\n%s", updates.String())
		})
	}
}
