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

// buildExpectedLeafVariants builds expected leaf variants by creating a fresh root with the same structure and transformations
// used to verify test cases. This ensures consistent parent setup and TreeContext state without manual construction.
func buildExpectedLeafVariants(t *testing.T, ctx context.Context, owner2 string, owner2Prio int32,
	loadConfigs func(*testing.T, context.Context, *RootEntry) error,
	explicitDeletes *sdcpb.PathSet) LeafVariantSlice {
	controller := gomock.NewController(t)
	defer controller.Finish()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)
	expectedTC := NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

	expectedRoot, err := NewTreeRoot(ctx, expectedTC)
	if err != nil {
		t.Error(err)
	}

	// Load configs based on test case needs
	if err := loadConfigs(t, ctx, expectedRoot); err != nil {
		t.Error(err)
	}

	// Apply nonrevertive info and explicit deletes (same as main test)
	expectedTC.AddNonRevertiveInfo(owner2, false)
	if explicitDeletes != nil {
		expectedTC.AddExplicitDeletes(owner2, owner2Prio, explicitDeletes)
	}

	// Finish insertion phase
	err = expectedRoot.FinishInsertionPhase(ctx)
	if err != nil {
		t.Error(err)
	}

	// Extract and return the leaf variants using GetByOwner
	lvs := LeafVariantSlice{}
	return expectedRoot.GetByOwner(owner2, lvs)
}

func TestExplicitDeleteVisitor_Visit(t *testing.T) {
	ctx := context.Background()
	owner1 := "owner1"
	owner1Prio := int32(500)
	owner2 := "owner2"
	owner2Prio := int32(50)

	tests := []struct {
		name                 string
		root                 func() *RootEntry
		owner                string
		priority             int32
		explicitDeletes      *sdcpb.PathSet
		wantErr              bool
		expectedLeafVariants func(tc *TreeContext) LeafVariantSlice
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
				tc := NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Error(err)
				}

				_, err = loadYgotStructIntoTreeRoot(ctx, config1(), root, owner1, owner1Prio, false, flagsNew)
				if err != nil {
					t.Error(err)
				}
				return root
			},
			expectedLeafVariants: func(tc *TreeContext) LeafVariantSlice {
				return buildExpectedLeafVariants(t, ctx, owner2, owner2Prio,
					func(t *testing.T, ctx context.Context, expectedRoot *RootEntry) error {
						_, err := loadYgotStructIntoTreeRoot(ctx, config1(), expectedRoot, owner1, owner1Prio, false, flagsNew)
						return err
					},
					nil) // no explicit deletes for this case
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
				tc := NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Error(err)
				}

				_, err = loadYgotStructIntoTreeRoot(ctx, config1(), root, owner1, owner1Prio, false, flagsExisting)
				if err != nil {
					t.Error(err)
				}

				_, err = loadYgotStructIntoTreeRoot(ctx, config1(), root, RunningIntentName, RunningValuesPrio, false, flagsExisting)
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
			expectedLeafVariants: func(tc *TreeContext) LeafVariantSlice {
				deletes := sdcpb.NewPathSet().AddPath(&sdcpb.Path{
					Elem: []*sdcpb.PathElem{
						sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
						sdcpb.NewPathElem("description", nil),
					},
				})
				return buildExpectedLeafVariants(t, ctx, owner2, owner2Prio,
					func(t *testing.T, ctx context.Context, expectedRoot *RootEntry) error {
						_, err := loadYgotStructIntoTreeRoot(ctx, config1(), expectedRoot, owner1, owner1Prio, false, flagsExisting)
						if err != nil {
							return err
						}
						_, err = loadYgotStructIntoTreeRoot(ctx, config1(), expectedRoot, RunningIntentName, RunningValuesPrio, false, flagsExisting)
						return err
					},
					deletes)
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
				tc := NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Error(err)
				}

				_, err = loadYgotStructIntoTreeRoot(ctx, config1(), root, owner1, owner1Prio, false, flagsExisting)
				if err != nil {
					t.Error(err)
				}

				_, err = loadYgotStructIntoTreeRoot(ctx, config1(), root, RunningIntentName, RunningValuesPrio, false, flagsExisting)
				if err != nil {
					t.Error(err)
				}

				loadYgotStructIntoTreeRoot(ctx, &sdcio_schema.Device{Interface: map[string]*sdcio_schema.SdcioModel_Interface{
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
			expectedLeafVariants: func(tc *TreeContext) LeafVariantSlice {
				deletes := sdcpb.NewPathSet().AddPath(&sdcpb.Path{
					Elem: []*sdcpb.PathElem{
						sdcpb.NewPathElem("interface", nil),
					},
				})
				return buildExpectedLeafVariants(t, ctx, owner2, owner2Prio,
					func(t *testing.T, ctx context.Context, expectedRoot *RootEntry) error {
						_, err := loadYgotStructIntoTreeRoot(ctx, config1(), expectedRoot, owner1, owner1Prio, false, flagsExisting)
						if err != nil {
							return err
						}

						_, err = loadYgotStructIntoTreeRoot(ctx, config1(), expectedRoot, RunningIntentName, RunningValuesPrio, false, flagsExisting)
						if err != nil {
							return err
						}

						_, err = loadYgotStructIntoTreeRoot(ctx, &sdcio_schema.Device{Interface: map[string]*sdcio_schema.SdcioModel_Interface{
							"ethernet-1/1": {
								Name:        ygot.String("ethernet-1/1"),
								Description: ygot.String("mydesc"),
							},
						}}, expectedRoot, owner2, owner2Prio, false, types.NewUpdateInsertFlags())
						return err
					},
					deletes)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.root()
			root.GetTreeContext().AddNonRevertiveInfo(owner2, false)
			root.GetTreeContext().AddExplicitDeletes(owner2, tt.priority, tt.explicitDeletes)

			err := root.FinishInsertionPhase(ctx)
			if err != nil {
				t.Error(err)
			}

			t.Log(root.String())

			lvs := LeafVariantSlice{}
			lvs = root.GetByOwner(owner2, lvs)

			expectedLeafVariants := tt.expectedLeafVariants(root.GetTreeContext())

			// Use Equal function for comparison - all test cases
			// Equal() handles sorting both slices internally
			equal, err := lvs.Equal(expectedLeafVariants)
			if err != nil {
				t.Logf("Comparison error: %v", err)
				t.FailNow()
			}
			if !equal {
				t.Logf("Leaf variants mismatch: expected %d entries, got %d", len(expectedLeafVariants), len(lvs))
				t.Logf("Expected:\n%s", expectedLeafVariants.String())
				t.Logf("Got:\n%s", lvs.String())
				t.FailNow()
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
