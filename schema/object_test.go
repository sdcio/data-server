package schema

import (
	"reflect"
	"sort"
	"testing"

	"github.com/iptecharch/schema-server/config"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/openconfig/goyang/pkg/yang"
)

func TestSchema_BuildPath(t *testing.T) {
	type fields struct {
		config *config.SchemaConfig
	}
	type args struct {
		pe []string
		p  *schemapb.Path
	}
	tests := []struct {
		name      string
		fields    fields
		testItems []struct {
			name    string
			args    args
			want    *schemapb.Path
			wantErr bool
		}
	}{
		{
			name: "SRL22.11.1",
			fields: fields{
				config: &config.SchemaConfig{
					Name:    "SRL-Native",
					Vendor:  "Nokia",
					Version: "22.11.1",
					Files: []string{
						"testdata/srl-latest-yang-models/srl_nokia/models",
					},
					Directories: []string{
						"testdata/srl-latest-yang-models/ietf",
					},
					Excludes: []string{".*tools.*"},
				},
			},
			testItems: []struct {
				name    string
				args    args
				want    *schemapb.Path
				wantErr bool
			}{
				{
					name: "path with key",
					args: args{
						pe: []string{"interface", "mgmt0"},
						p:  &schemapb.Path{},
					},
					want: &schemapb.Path{Elem: []*schemapb.PathElem{
						{
							Name: "interface",
							Key:  map[string]string{"name": "mgmt0"},
						},
					}},
					wantErr: false,
				},
				{
					name: "path with intermediate key",
					args: args{
						pe: []string{"interface", "mgmt0", "subinterface"},
						p:  &schemapb.Path{},
					},
					want: &schemapb.Path{Elem: []*schemapb.PathElem{
						{
							Name: "interface",
							Key:  map[string]string{"name": "mgmt0"},
						},
						{
							Name: "subinterface",
						},
					}},
					wantErr: false,
				},
				{
					name: "path with 2 intermediate key",
					args: args{
						pe: []string{"interface", "mgmt0", "subinterface", "0", "admin-state"},
						p:  &schemapb.Path{},
					},
					want: &schemapb.Path{Elem: []*schemapb.PathElem{
						{
							Name: "interface",
							Key:  map[string]string{"name": "mgmt0"},
						},
						{
							Name: "subinterface",
							Key:  map[string]string{"index": "0"},
						},
						{
							Name: "admin-state",
						},
					}},
					wantErr: false,
				},
				{
					name: "path with choice/case",
					args: args{
						pe: []string{"acl", "cpm-filter", "ipv4-filter", "entry", "0", "action", "accept"},
						p:  &schemapb.Path{},
					},
					want: &schemapb.Path{Elem: []*schemapb.PathElem{
						{
							Name: "acl",
						},
						{
							Name: "cpm-filter",
						},
						{
							Name: "ipv4-filter",
						},
						{
							Name: "entry",
							Key:  map[string]string{"sequence-id": "0"},
						},
						{
							Name: "action",
						},
						{
							Name: "accept",
						},
					}},
					wantErr: false,
				},
			},
		},
		{
			name: "dummy",
			fields: fields{
				config: &config.SchemaConfig{
					Name:    "dummy",
					Vendor:  "dummy_vendor",
					Version: "0.0.0",
					Files: []string{
						"testdata/dummy",
					},
					Directories: []string{},
					Excludes:    []string{},
				},
			},
			testItems: []struct {
				name    string
				args    args
				want    *schemapb.Path
				wantErr bool
			}{
				{
					name: "single path elem",
					args: args{
						pe: []string{"foo"},
						p:  &schemapb.Path{},
					},
					want: &schemapb.Path{Elem: []*schemapb.PathElem{
						{
							Name: "foo",
						},
					}},
					wantErr: false,
				},
				{
					name: "two path elem with key",
					args: args{
						pe: []string{"foo", "bar", "kv1"},
						p:  &schemapb.Path{},
					},
					want: &schemapb.Path{Elem: []*schemapb.PathElem{
						{
							Name: "foo",
						},
						{
							Name: "bar",
							Key:  map[string]string{"k1": "kv1"},
						},
					}},
					wantErr: false,
				},
				{
					name: "two path elem with 2 keys",
					args: args{
						pe: []string{"foo", "bar", "kv1", "kv2"},
						p:  &schemapb.Path{},
					},
					want: &schemapb.Path{Elem: []*schemapb.PathElem{
						{
							Name: "foo",
						},
						{
							Name: "bar",
							Key: map[string]string{
								"k1": "kv1",
								"k2": "kv2",
							},
						},
					}},
					wantErr: false,
				},
				{
					name: "two path elem with 2 keys and sub leaf",
					args: args{
						pe: []string{"foo", "bar", "kv1", "kv2", "attr1"},
						p:  &schemapb.Path{},
					},
					want: &schemapb.Path{Elem: []*schemapb.PathElem{
						{
							Name: "foo",
						},
						{
							Name: "bar",
							Key: map[string]string{
								"k1": "kv1",
								"k2": "kv2",
							},
						},
						{
							Name: "attr1",
						},
					}},
					wantErr: false,
				},
				{
					name: "two path elem with 2 keys and sub container",
					args: args{
						pe: []string{"foo", "bar", "kv1", "kv2", "subbar"},
						p:  &schemapb.Path{},
					},
					want: &schemapb.Path{Elem: []*schemapb.PathElem{
						{
							Name: "foo",
						},
						{
							Name: "bar",
							Key: map[string]string{
								"k1": "kv1",
								"k2": "kv2",
							},
						},
						{
							Name: "subbar",
						},
					}},
					wantErr: false,
				},
				{
					name: "two path elem with 2 keys and sub container and leaf",
					args: args{
						pe: []string{"foo", "bar", "kv1", "kv2", "subbar", "subattr1"},
						p:  &schemapb.Path{},
					},
					want: &schemapb.Path{Elem: []*schemapb.PathElem{
						{
							Name: "foo",
						},
						{
							Name: "bar",
							Key: map[string]string{
								"k1": "kv1",
								"k2": "kv2",
							},
						},
						{
							Name: "subbar",
						},
						{
							Name: "subattr1",
						},
					}},
					wantErr: false,
				},
				{
					name: "path with choice/case 1",
					args: args{
						pe: []string{"foo", "foo1", "case1-container", "cas1_leaf"},
						p:  &schemapb.Path{},
					},
					want: &schemapb.Path{Elem: []*schemapb.PathElem{
						{
							Name: "foo",
						},
						{
							Name: "foo1",
						},
						{
							Name: "case1-container",
						},
						{
							Name: "cas1_leaf",
						},
					}},
					wantErr: false,
				},
				{
					name: "path with choice/case 2",
					args: args{
						pe: []string{"foo", "foo1", "case2-container", "cas2_leaf"},
						p:  &schemapb.Path{},
					},
					want: &schemapb.Path{Elem: []*schemapb.PathElem{
						{
							Name: "foo",
						},
						{
							Name: "foo1",
						},
						{
							Name: "case2-container",
						},
						{
							Name: "cas2_leaf",
						},
					}},
					wantErr: false,
				},
				{
					name: "path with choice/case 3",
					args: args{
						pe: []string{"foo", "foo1", "cas3-leaf"},
						p:  &schemapb.Path{},
					},
					want: &schemapb.Path{Elem: []*schemapb.PathElem{
						{
							Name: "foo",
						},
						{
							Name: "foo1",
						},
						{
							Name: "cas3-leaf",
						},
					}},
					wantErr: false,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc, err := NewSchema(tt.fields.config)
			if err != nil {
				t.Errorf("%s: failed to create schema", err)
			}
			t.Logf("%s: schema parsed", tt.name)
			for _, ti := range tt.testItems {
				if err := sc.BuildPath(ti.args.pe, ti.args.p); (err != nil) != ti.wantErr {
					t.Errorf("Schema.BuildPath() error = %v, wantErr %v", err, ti.wantErr)
				}
				if !comparePaths(ti.args.p, ti.want) {
					t.Logf("%s:", ti.name)
					t.Logf("got : %v", ti.args.p)
					t.Logf("want: %v", ti.want)
					t.Errorf("%s: failed", ti.name)
				}
			}
		})
	}
}

func comparePathElem(pe1, pe2 *schemapb.PathElem) bool {
	if pe1 == nil {
		return pe2 == nil
	}
	if pe2 == nil {
		return pe1 == nil
	}
	if pe1.GetName() != pe2.GetName() {
		return false
	}
	lPe1Keys := len(pe1.GetKey())
	lPe2Keys := len(pe2.GetKey())
	if lPe1Keys != lPe2Keys {
		return false
	}
	for k, v := range pe1.GetKey() {
		if pe2.GetKey()[k] != v {
			return false
		}
	}
	return true
}

func comparePaths(p1, p2 *schemapb.Path) bool {
	if p1 == nil {
		return p2 == nil
	}
	if p2 == nil {
		return p1 == nil
	}
	if p1.GetOrigin() != p2.GetOrigin() {
		return false
	}
	if p1.GetTarget() != p2.GetTarget() {
		return false
	}
	if len(p1.GetElem()) != len(p2.GetElem()) {
		return false
	}
	for i, pe := range p1.GetElem() {
		if !comparePathElem(pe, p2.GetElem()[i]) {
			return false
		}
	}
	return true
}

func Test_getChildren(t *testing.T) {
	type args struct {
		e *yang.Entry
	}
	tests := []struct {
		name string
		args args
		want []*yang.Entry
	}{
		{
			name: "leaf",
			args: args{
				e: &yang.Entry{
					Node:        &yang.Leaf{},
					Name:        "",
					Description: "",
					Default:     []string{},
					Units:       "",
					Errors:      []error{},
					Kind:        0,
					Config:      0,
					Prefix:      &yang.Value{},
					Mandatory:   0,
					Dir:         map[string]*yang.Entry{},
					Key:         "",
					Type:        &yang.YangType{},
					Exts:        []*yang.Statement{},
					ListAttr:    &yang.ListAttr{},
					RPC:         &yang.RPCEntry{},
					Identities:  []*yang.Identity{},
					Augments:    []*yang.Entry{},
					Augmented:   []*yang.Entry{},
					Deviations:  []*yang.DeviatedEntry{},
					//Deviate:     map[yang.deviationType][]*yang.Entry{},
					Uses:       []*yang.UsesStmt{},
					Extra:      map[string][]interface{}{},
					Annotation: map[string]interface{}{},
				},
			},
			want: []*yang.Entry{},
		},
		{
			name: "leaflist",
			args: args{
				e: &yang.Entry{
					Node:        &yang.LeafList{},
					Name:        "",
					Description: "",
					Default:     []string{},
					Units:       "",
					Errors:      []error{},
					Kind:        0,
					Config:      0,
					Prefix:      &yang.Value{},
					Mandatory:   0,
					Dir:         map[string]*yang.Entry{},
					Key:         "",
					Type:        &yang.YangType{},
					Exts:        []*yang.Statement{},
					ListAttr:    &yang.ListAttr{},
					RPC:         &yang.RPCEntry{},
					Identities:  []*yang.Identity{},
					Augments:    []*yang.Entry{},
					Augmented:   []*yang.Entry{},
					Deviations:  []*yang.DeviatedEntry{},
					//Deviate:     map[yang.deviationType][]*yang.Entry{},
					Uses:       []*yang.UsesStmt{},
					Extra:      map[string][]interface{}{},
					Annotation: map[string]interface{}{},
				},
			},
			want: []*yang.Entry{},
		},
		{
			name: "container",
			args: args{
				e: &yang.Entry{
					Node:        &yang.Container{},
					Name:        "",
					Description: "",
					Default:     []string{},
					Units:       "",
					Errors:      []error{},
					Kind:        0,
					Config:      0,
					Prefix:      &yang.Value{},
					Mandatory:   0,
					Dir: map[string]*yang.Entry{
						"l1": {Node: &yang.Leaf{}, Name: "l1"},
						"l2": {Node: &yang.Leaf{}, Name: "l2"},
						"c1": {Node: &yang.Container{}, Kind: yang.DirectoryEntry, Name: "c1"},
					},
					Key:        "",
					Type:       &yang.YangType{},
					Exts:       []*yang.Statement{},
					ListAttr:   &yang.ListAttr{},
					RPC:        &yang.RPCEntry{},
					Identities: []*yang.Identity{},
					Augments:   []*yang.Entry{},
					Augmented:  []*yang.Entry{},
					Deviations: []*yang.DeviatedEntry{},
					//Deviate:     map[yang.deviationType][]*yang.Entry{},
					Uses:       []*yang.UsesStmt{},
					Extra:      map[string][]interface{}{},
					Annotation: map[string]interface{}{},
				},
			},
			want: []*yang.Entry{
				{Node: &yang.Leaf{}, Name: "l1"},
				{Node: &yang.Leaf{}, Name: "l2"},
				{Node: &yang.Container{}, Kind: yang.DirectoryEntry, Name: "c1"},
			},
		},
		{
			name: "container with choice",
			args: args{
				e: &yang.Entry{
					Node:        &yang.Container{},
					Name:        "",
					Description: "",
					Default:     []string{},
					Units:       "",
					Errors:      []error{},
					Kind:        yang.DirectoryEntry,
					Config:      0,
					Prefix:      &yang.Value{},
					Mandatory:   0,
					Dir: map[string]*yang.Entry{
						"l1": {Node: &yang.Leaf{}, Name: "l1"},
						"l2": {Node: &yang.Leaf{}, Name: "l2"},
						"c1": {
							Node: &yang.Container{},
							Kind: yang.DirectoryEntry,
							Name: "c1",
						},
						"ch1": {Node: &yang.Choice{}, Name: "ch1", Kind: yang.ChoiceEntry, Dir: map[string]*yang.Entry{
							"c2": {
								Node: &yang.Container{},
								Kind: yang.DirectoryEntry,
								Name: "c2",
							},
						}},
					},
					Key:        "",
					Type:       &yang.YangType{},
					Exts:       []*yang.Statement{},
					ListAttr:   nil,
					RPC:        &yang.RPCEntry{},
					Identities: []*yang.Identity{},
					Augments:   []*yang.Entry{},
					Augmented:  []*yang.Entry{},
					Deviations: []*yang.DeviatedEntry{},
					//Deviate:     map[yang.deviationType][]*yang.Entry{},
					Uses:       []*yang.UsesStmt{},
					Extra:      map[string][]interface{}{},
					Annotation: map[string]interface{}{},
				},
			},
			want: []*yang.Entry{
				{Node: &yang.Leaf{}, Name: "l1"},
				{Node: &yang.Leaf{}, Name: "l2"},
				{Node: &yang.Container{}, Kind: yang.DirectoryEntry, Name: "c1"},
				{Node: &yang.Container{}, Kind: yang.DirectoryEntry, Name: "c2"},
			},
		},
		{
			name: "container with choice and case",
			args: args{
				e: &yang.Entry{
					Node:        &yang.Container{},
					Name:        "",
					Description: "",
					Default:     []string{},
					Units:       "",
					Errors:      []error{},
					Kind:        yang.DirectoryEntry,
					Config:      0,
					Prefix:      &yang.Value{},
					Mandatory:   0,
					Dir: map[string]*yang.Entry{
						"l1": {Node: &yang.Leaf{}, Name: "l1"},
						"l2": {Node: &yang.Leaf{}, Name: "l2"},
						"c3": {
							Node: &yang.Container{},
							Kind: yang.DirectoryEntry,
							Name: "c3",
						},
						"ch1": {Node: &yang.Choice{}, Name: "ch1", Kind: yang.ChoiceEntry, Dir: map[string]*yang.Entry{
							"case1": {
								Node: &yang.Container{},
								Kind: yang.CaseEntry,
								Name: "c1",
							},
							"case2": {
								Node: &yang.Container{},
								Kind: yang.CaseEntry,
								Name: "c2",
							},
						}},
					},
					Key:        "",
					Type:       &yang.YangType{},
					Exts:       []*yang.Statement{},
					ListAttr:   nil,
					RPC:        &yang.RPCEntry{},
					Identities: []*yang.Identity{},
					Augments:   []*yang.Entry{},
					Augmented:  []*yang.Entry{},
					Deviations: []*yang.DeviatedEntry{},
					//Deviate:     map[yang.deviationType][]*yang.Entry{},
					Uses:       []*yang.UsesStmt{},
					Extra:      map[string][]interface{}{},
					Annotation: map[string]interface{}{},
				},
			},
			want: []*yang.Entry{
				{Node: &yang.Leaf{}, Name: "l1"},
				{Node: &yang.Leaf{}, Name: "l2"},
				{Node: &yang.Container{}, Kind: yang.CaseEntry, Name: "c1"},
				{Node: &yang.Container{}, Kind: yang.CaseEntry, Name: "c2"},
				{Node: &yang.Container{}, Kind: yang.DirectoryEntry, Name: "c3"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getChildren(tt.args.e)
			sort.Slice(got, sortFn(got))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getChildren() = %v, want %v", got, tt.want)
				for _, g := range got {
					t.Errorf("got : %T | %s", g.Node, g.Name)
				}
				for _, w := range tt.want {
					t.Errorf("want: %T | %s", w.Node, w.Name)
				}
			}
		})
	}
}
