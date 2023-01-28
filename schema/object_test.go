package schema

import (
	"testing"

	"github.com/iptecharch/schema-server/config"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
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
