package tree

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/openconfig/ygot/ygot"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestRootEntry_TreeExport(t *testing.T) {
	owner1 := "owner1"
	owner2 := "owner2"
	tc := NewTreeContext(nil, owner1)

	type args struct {
		owner    string
		priority int32
	}
	tests := []struct {
		name                  string
		sharedEntryAttributes func() *sharedEntryAttributes
		args                  args
		want                  func(*testing.T) *tree_persist.Intent
		wantErr               bool
	}{
		{
			name: "Only LeafVariant at Root",
			sharedEntryAttributes: func() *sharedEntryAttributes {
				result := &sharedEntryAttributes{
					parent:       nil,
					pathElemName: "",
					childs:       newChildMap(),
					childsMutex:  sync.RWMutex{},
					schemaMutex:  sync.RWMutex{},
					cacheMutex:   sync.Mutex{},
					treeContext:  tc,
				}
				result.leafVariants = newLeafVariants(tc, result)

				result.leafVariants.Add(
					NewLeafEntry(
						types.NewUpdate(nil,
							&sdcpb.TypedValue{
								Value: &sdcpb.TypedValue_StringVal{StringVal: "Value"},
							}, 500, owner1, 0,
						),
						types.NewUpdateInsertFlags(), result),
				)

				return result
			},
			args: args{
				owner:    owner1,
				priority: 500,
			},
			want: func(t *testing.T) *tree_persist.Intent {
				lv, err := proto.Marshal(&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "Value"}})
				if err != nil {
					t.Fatal(err)
				}

				result := &tree_persist.Intent{
					IntentName: owner1,
					Priority:   500,
					Root: &tree_persist.TreeElement{
						Name:        "",
						Childs:      []*tree_persist.TreeElement{},
						LeafVariant: lv,
					},
				}
				return result
			},
		},
		{
			name: "LeafVariants at Childs",
			sharedEntryAttributes: func() *sharedEntryAttributes {
				// create root sharedEntryAttributes
				result := &sharedEntryAttributes{
					parent:       nil,
					pathElemName: "",
					childs:       newChildMap(),
					childsMutex:  sync.RWMutex{},
					schemaMutex:  sync.RWMutex{},
					cacheMutex:   sync.Mutex{},
					treeContext:  tc,
				}
				result.leafVariants = newLeafVariants(tc, result)

				// create /interface sharedEntryAttributes
				interf := &sharedEntryAttributes{
					parent:       result,
					pathElemName: "interface",
					childs:       newChildMap(),
					childsMutex:  sync.RWMutex{},
					schemaMutex:  sync.RWMutex{},
					cacheMutex:   sync.Mutex{},
				}
				interf.leafVariants = newLeafVariants(tc, interf)
				// add interf to result (root)
				result.childs.Add(interf)

				// add interface LeafVariant
				interf.leafVariants.Add(
					NewLeafEntry(
						types.NewUpdate(nil,
							&sdcpb.TypedValue{
								Value: &sdcpb.TypedValue_StringVal{StringVal: "Value"},
							}, 500, owner1, 0,
						),
						types.NewUpdateInsertFlags(), result),
				)

				return result
			},
			args: args{
				owner:    owner1,
				priority: 500,
			},
			want: func(t *testing.T) *tree_persist.Intent {
				lv, err := proto.Marshal(&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "Value"}})
				if err != nil {
					t.Fatal(err)
				}

				result := &tree_persist.Intent{
					IntentName: owner1,
					Priority:   500,
					Root: &tree_persist.TreeElement{
						Name: "",
						Childs: []*tree_persist.TreeElement{
							{
								Name:        "interface",
								LeafVariant: lv,
							},
						},
					},
				}
				return result
			},
		},
		{
			name: "LeafVariants at Childs including other owners",
			sharedEntryAttributes: func() *sharedEntryAttributes {
				// create root sharedEntryAttributes
				result := &sharedEntryAttributes{
					parent:       nil,
					pathElemName: "",
					childs:       newChildMap(),
					childsMutex:  sync.RWMutex{},
					schemaMutex:  sync.RWMutex{},
					cacheMutex:   sync.Mutex{},
					treeContext:  tc,
				}
				result.leafVariants = newLeafVariants(tc, result)

				// create /interface sharedEntryAttributes
				interf := &sharedEntryAttributes{
					parent:       result,
					pathElemName: "interface",
					childs:       newChildMap(),
					childsMutex:  sync.RWMutex{},
					schemaMutex:  sync.RWMutex{},
					cacheMutex:   sync.Mutex{},
				}
				interf.leafVariants = newLeafVariants(tc, interf)
				// add interf to result (root)
				result.childs.Add(interf)

				// add interface LeafVariant
				interf.leafVariants.Add(
					NewLeafEntry(
						types.NewUpdate(nil,
							&sdcpb.TypedValue{
								Value: &sdcpb.TypedValue_StringVal{StringVal: "Value"},
							}, 500, owner1, 0,
						),
						types.NewUpdateInsertFlags(), result),
				)
				// add interface LeafVariant
				interf.leafVariants.Add(
					NewLeafEntry(
						types.NewUpdate(nil,
							&sdcpb.TypedValue{
								Value: &sdcpb.TypedValue_StringVal{StringVal: "OtherValue"},
							}, 50, owner2, 0,
						),
						types.NewUpdateInsertFlags(), result),
				)

				// create /system sharedEntryAttributes
				system := &sharedEntryAttributes{
					parent:       result,
					pathElemName: "system",
					childs:       newChildMap(),
					childsMutex:  sync.RWMutex{},
					schemaMutex:  sync.RWMutex{},
					cacheMutex:   sync.Mutex{},
				}
				system.leafVariants = newLeafVariants(tc, system)
				// add interf to result (root)
				result.childs.Add(system)

				// add interface LeafVariant
				interf.leafVariants.Add(
					NewLeafEntry(
						types.NewUpdate(nil,
							&sdcpb.TypedValue{
								Value: &sdcpb.TypedValue_StringVal{StringVal: "Value"},
							}, 50, owner2, 0,
						),
						types.NewUpdateInsertFlags(), result),
				)

				return result
			},
			args: args{
				owner:    owner1,
				priority: 500,
			},
			want: func(t *testing.T) *tree_persist.Intent {
				lv, err := proto.Marshal(&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "Value"}})
				if err != nil {
					t.Error(err)
				}

				result := &tree_persist.Intent{
					IntentName: owner1,
					Priority:   500,
					Root: &tree_persist.TreeElement{
						Name: "",
						Childs: []*tree_persist.TreeElement{
							{
								Name:        "interface",
								LeafVariant: lv,
							},
						},
					},
				}
				return result
			},
		},
		{
			name: "Export Non-Existing Owner",
			sharedEntryAttributes: func() *sharedEntryAttributes {
				// create root sharedEntryAttributes
				result := &sharedEntryAttributes{
					parent:       nil,
					pathElemName: "",
					childs:       newChildMap(),
					childsMutex:  sync.RWMutex{},
					schemaMutex:  sync.RWMutex{},
					cacheMutex:   sync.Mutex{},
					treeContext:  tc,
				}
				result.leafVariants = newLeafVariants(tc, result)

				// create /interface sharedEntryAttributes
				interf := &sharedEntryAttributes{
					parent:       result,
					pathElemName: "interface",
					childs:       newChildMap(),
					childsMutex:  sync.RWMutex{},
					schemaMutex:  sync.RWMutex{},
					cacheMutex:   sync.Mutex{},
				}
				interf.leafVariants = newLeafVariants(tc, interf)
				// add interf to result (root)
				result.childs.Add(interf)

				// add interface LeafVariant
				interf.leafVariants.Add(
					NewLeafEntry(
						types.NewUpdate(nil,
							&sdcpb.TypedValue{
								Value: &sdcpb.TypedValue_StringVal{StringVal: "Value"},
							}, 500, owner1, 0,
						),
						types.NewUpdateInsertFlags(), result),
				)

				return result
			},
			args: args{
				owner:    owner2,
				priority: 50,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RootEntry{
				sharedEntryAttributes: tt.sharedEntryAttributes(),
				explicitDeletes:       NewDeletePaths(),
			}
			got, err := r.TreeExport(tt.args.owner, tt.args.priority)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RootEntry.TreeExport() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && !proto.Equal(got, tt.want(t)) {
				t.Fatalf("RootEntry.TreeExport() = %v, want %v", got, tt.want(t))
			}
		})
	}
}

func TestRootEntry_DeleteSubtreePaths(t *testing.T) {
	owner1 := "owner1"

	type args struct {
		deletes    types.DeleteEntriesList
		intentName string
	}
	tests := []struct {
		name string
		re   func() ygot.GoStruct
		args args
	}{
		{
			name: "Delete one",
			args: args{
				deletes:    types.DeleteEntriesList{},
				intentName: owner1,
			},
			re: func() ygot.GoStruct {
				return &sdcio_schema.Device{
					Doublekey: map[sdcio_schema.SdcioModel_Doublekey_Key]*sdcio_schema.SdcioModel_Doublekey{
						{
							Key1: "k1.1",
							Key2: "k1.2",
						}: {
							Key1:    ygot.String("k1.1"),
							Key2:    ygot.String("k1.2"),
							Mandato: ygot.String("TheMandatoryValue1"),
							Cont: &sdcio_schema.SdcioModel_Doublekey_Cont{
								Value1: ygot.String("containerval1.1"),
								Value2: ygot.String("containerval1.2"),
							},
						},
						{
							Key1: "k2.1",
							Key2: "k2.2",
						}: {
							Key1:    ygot.String("k2.1"),
							Key2:    ygot.String("k2.2"),
							Mandato: ygot.String("TheMandatoryValue2"),
							Cont: &sdcio_schema.SdcioModel_Doublekey_Cont{
								Value1: ygot.String("containerval2.1"),
								Value2: ygot.String("containerval2.2"),
							},
						},
						{
							Key1: "k1.1",
							Key2: "k1.3",
						}: {
							Key1:    ygot.String("k1.1"),
							Key2:    ygot.String("k1.3"),
							Mandato: ygot.String("TheMandatoryValue1"),
							Cont: &sdcio_schema.SdcioModel_Doublekey_Cont{
								Value1: ygot.String("containerval1.1"),
								Value2: ygot.String("containerval1.2"),
							},
						},
					},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

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
				t.Fatal(err)
			}

			err = testhelper.LoadYgotStructIntoTreeRoot(ctx, tt.re(), root, owner1, 500, false, flagsNew)
			if err != nil {
				t.Fatal(err)
			}

			err = root.FinishInsertionPhase(ctx)
			if err != nil {
				t.Fatal(err)
			}

			fmt.Println(root.String())

			err = root.DeleteBranchPaths(ctx, tt.args.deletes, tt.args.intentName)
			if err != nil {
				t.Fatal(err)
			}

			fmt.Println(root.String())

		})
	}
}

func TestRootEntry_AddUpdatesRecursive(t *testing.T) {
	ctx := context.Background()
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)
	tc := NewTreeContext(scb, "intent1")

	type fields struct {
		sharedEntryAttributes func(t *testing.T) *sharedEntryAttributes
	}
	type args struct {
		pau   []*types.PathAndUpdate
		flags *types.UpdateInsertFlags
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
		want    func(t *testing.T) *RootEntry
	}{
		{
			name: "simple add",
			fields: fields{
				sharedEntryAttributes: func(t *testing.T) *sharedEntryAttributes {
					s, err := newSharedEntryAttributes(ctx, nil, "", tc)
					if err != nil {
						t.Fatal(err)
					}
					schema, err := tc.schemaClient.GetSchemaSdcpbPath(ctx, nil)
					if err != nil {
						t.Fatal(err)
					}
					s.schema = schema.GetSchema()
					s.leafVariants = newLeafVariants(tc, s)
					return s
				},
			},
			args: args{
				pau: []*types.PathAndUpdate{
					types.NewPathAndUpdate(
						&sdcpb.Path{
							Elem: []*sdcpb.PathElem{
								sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
								sdcpb.NewPathElem("description", nil),
							},
						},
						types.NewUpdate(nil, testhelper.GetStringTvProto("test"), *proto.Int32(5), "owner1", 0),
					),
					types.NewPathAndUpdate(
						&sdcpb.Path{
							Elem: []*sdcpb.PathElem{
								sdcpb.NewPathElem("network-instance", map[string]string{
									"name": "ni1",
								}),
								sdcpb.NewPathElem("protocol", nil),
								sdcpb.NewPathElem("bgp", nil),
							},
						},
						types.NewUpdate(nil, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_EmptyVal{EmptyVal: &emptypb.Empty{}}}, *proto.Int32(5), "owner1", 0),
					),
				},
				flags: types.NewUpdateInsertFlags(),
			},
			want: func(t *testing.T) *RootEntry {
				s, err := newSharedEntryAttributes(ctx, nil, "", tc)
				if err != nil {
					t.Fatal(err)
				}

				jsonByte := []byte(`{
	"interface": [
		{
			"name": "ethernet-1/1",
			"description": "test"
		}
	],
	"network-instance": [
		{
			"name": "ni1",
			"protocol":	{
					"bgp": {}
			}
		}
	]
}`)
				var jsonAny any
				err = json.Unmarshal(jsonByte, &jsonAny)
				if err != nil {
					t.Fatal(err)
				}

				vpf := pool.NewSharedTaskPool(ctx, runtime.NumCPU())
				err = s.ImportConfig(ctx, jsonImporter.NewJsonTreeImporter(jsonAny, "owner1", 5, false), types.NewUpdateInsertFlags(), vpf)
				if err != nil {
					t.Fatal(err)
				}

				return &RootEntry{sharedEntryAttributes: s}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RootEntry{
				sharedEntryAttributes: tt.fields.sharedEntryAttributes(t),
			}
			if err := r.AddUpdatesRecursive(ctx, tt.args.pau, tt.args.flags); (err != nil) != tt.wantErr {
				t.Errorf("RootEntry.AddUpdatesRecursive() error = %v, wantErr %v", err, tt.wantErr)
			}

			if diff := cmp.Diff(tt.want(t).String(), r.String()); diff != "" {
				t.Fatalf("mismatch (-want +got)\n%s", diff)
			}
		})
	}
}

func TestRootEntry_GetUpdatesForOwner(t *testing.T) {
	ctx := context.Background()
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	owner1 := "owner1"
	owner2 := "owner2"

	tests := []struct {
		name      string
		rootEntry func(t *testing.T) *RootEntry
		owner     string
		want      func(t *testing.T) *RootEntry
	}{
		{
			name: "One",
			rootEntry: func(t *testing.T) *RootEntry {
				tc := NewTreeContext(scb, "intent1")
				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}
				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, config1(), root, owner1, 500, false, flagsNew)
				if err != nil {
					t.Fatal(err)
				}
				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, config2(), root, owner2, 400, false, flagsNew)
				if err != nil {
					t.Fatal(err)
				}
				err = root.FinishInsertionPhase(ctx)
				if err != nil {
					t.Fatal(err)
				}
				return root
			},
			owner: owner1,
			want: func(t *testing.T) *RootEntry {
				tc := NewTreeContext(scb, "intent1")
				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}
				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, config1(), root, owner1, 500, false, flagsNew)
				if err != nil {
					t.Fatal(err)
				}
				err = root.FinishInsertionPhase(ctx)
				if err != nil {
					t.Fatal(err)
				}
				return root
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.rootEntry(t)
			got := root.GetUpdatesForOwner(tt.owner).ToPathAndUpdateSlice()

			tc := NewTreeContext(scb, "intent1")
			resultRoot, err := NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatal(err)
			}
			err = resultRoot.AddUpdatesRecursive(ctx, got, flagsNew)
			if err != nil {
				t.Fatal(err)
			}

			err = resultRoot.FinishInsertionPhase(ctx)
			if err != nil {
				t.Fatal(err)
			}

			wantStr := tt.want(t).String()

			if diff := cmp.Diff(wantStr, resultRoot.String()); diff != "" {
				t.Errorf("GetUpdatesForOwner mismatch (-want +got):\n%s", diff)

				t.Logf("Want:\n%s", wantStr)
				t.Logf("Got:\n%s", resultRoot.String())
			}
		})
	}
}
