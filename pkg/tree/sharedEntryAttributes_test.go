package tree

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/openconfig/ygot/ygot"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	"go.uber.org/mock/gomock"
)

func Test_sharedEntryAttributes_checkAndCreateKeysAsLeafs(t *testing.T) {
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	tc := NewTreeContext(scb, "intent1")

	root, err := NewTreeRoot(ctx, tc)
	if err != nil {
		t.Error(err)
	}

	flags := types.NewUpdateInsertFlags()
	flags.SetNewFlag()

	prio := int32(5)
	intentName := "intent1"

	_, err = root.AddUpdateRecursive(ctx, types.NewUpdate(types.PathSlice{"interface", "ethernet-1/1", "description"}, testhelper.GetStringTvProto("MyDescription"), prio, intentName, 0), flags)
	if err != nil {
		t.Error(err)
	}

	_, err = root.AddUpdateRecursive(ctx, types.NewUpdate([]string{"doublekey", "k1.1", "k1.3", "mandato"}, testhelper.GetStringTvProto("TheMandatoryValue1"), prio, intentName, 0), flags)
	if err != nil {
		t.Error(err)
	}

	t.Log(root.String())

	fmt.Println(root.String())
	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		t.Error(err)
	}
	fmt.Println(root.String())

	// TODO: check the result
}

func Test_sharedEntryAttributes_DeepCopy(t *testing.T) {
	owner1 := "owner1"
	tests := []struct {
		name string
		root func() *RootEntry
	}{
		{
			name: "just rootEntry",
			root: func() *RootEntry {
				tc := NewTreeContext(nil, owner1)
				r := &RootEntry{
					&sharedEntryAttributes{
						pathElemName:     "__root__",
						childs:           newChildMap(),
						childsMutex:      sync.RWMutex{},
						choicesResolvers: choiceResolvers{},
						parent:           nil,
						treeContext:      tc,
					},
				}
				r.leafVariants = newLeafVariants(tc, r.sharedEntryAttributes)
				return r
			},
		},
		{
			name: "more complex tree",
			root: func() *RootEntry {
				// create a gomock controller
				controller := gomock.NewController(t)
				defer controller.Finish()

				ctx := context.Background()

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

				err = root.ImportConfig(ctx, types.PathSlice{}, jsonImporter.NewJsonTreeImporter(jsonConfAny), owner1, 500, newFlag)
				if err != nil {
					t.Error(err)
				}

				err = root.FinishInsertionPhase(ctx)
				if err != nil {
					t.Error(err)
				}
				return root
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.root()

			ctx := context.Background()

			newRoot, err := root.DeepCopy(ctx)
			if err != nil {
				return
			}

			if diff := cmp.Diff(root.String(), newRoot.String()); diff != "" {
				t.Fatalf("mismatching trees (-want +got)\n%s", diff)
			}
		})
	}
}

// func Test_sharedEntryAttributes_DeleteSubtree(t *testing.T) {

// 	type args struct {
// 		relativePath types.PathSlice
// 		owner        string
// 	}
// 	tests := []struct {
// 		name                  string
// 		sharedEntryAttributes func(t *testing.T) *sharedEntryAttributes
// 		args                  args
// 		want                  bool
// 		wantErr               bool
// 	}{
// 		{
// 			name: "one",
// 			sharedEntryAttributes: func(t *testing.T) *sharedEntryAttributes {

// 				return nil
// 			},
// 			args:    args{},
// 			want:    true,
// 			wantErr: false,
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			s := tt.sharedEntryAttributes(t)
// 			got, err := s.DeleteSubtree(tt.args.relativePath, tt.args.owner)
// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("sharedEntryAttributes.DeleteSubtree() error = %v, wantErr %v", err, tt.wantErr)
// 				return
// 			}
// 			if got != tt.want {
// 				t.Errorf("sharedEntryAttributes.DeleteSubtree() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }
