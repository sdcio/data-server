package tree

import (
	"sync"
	"testing"

	"github.com/sdcio/data-server/pkg/tree/tree_persist"
	"github.com/sdcio/data-server/pkg/tree/types"
	schema_server "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

func TestRootEntry_TreeExport(t *testing.T) {
	owner1 := "owner1"
	tc := NewTreeContext(nil, owner1)

	type fields struct {
		sharedEntryAttributes *sharedEntryAttributes
	}
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
				}
				result.leafVariants = newLeafVariants(tc, result)

				result.leafVariants.Add(
					NewLeafEntry(
						types.NewUpdate(types.PathSlice{},
							&schema_server.TypedValue{
								Value: &schema_server.TypedValue_StringVal{StringVal: "Value"},
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
				lv, err := proto.Marshal(&schema_server.TypedValue{Value: &schema_server.TypedValue_StringVal{StringVal: "Value"}})
				if err != nil {
					t.Error(err)
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
						types.NewUpdate(types.PathSlice{},
							&schema_server.TypedValue{
								Value: &schema_server.TypedValue_StringVal{StringVal: "Value"},
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
				lv, err := proto.Marshal(&schema_server.TypedValue{Value: &schema_server.TypedValue_StringVal{StringVal: "Value"}})
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RootEntry{
				sharedEntryAttributes: tt.sharedEntryAttributes(),
			}
			got, err := r.TreeExport(tt.args.owner, tt.args.priority)
			if (err != nil) != tt.wantErr {
				t.Errorf("RootEntry.TreeExport() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !proto.Equal(got, tt.want(t)) {
				t.Errorf("RootEntry.TreeExport() = %v, want %v", got, tt.want(t))
			}
		})
	}
}
