// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package jsonbuilder

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
)

func TestJsonBuilder_AddPath(t *testing.T) {
	tests := []struct {
		name string
		pvs  []struct {
			path  []*sdcpb.PathElem
			value string
		}
		want    *JEntry
		wantErr bool
	}{
		{
			name:    "Single Element",
			wantErr: false,
			pvs: []struct {
				path  []*sdcpb.PathElem
				value string
			}{
				{
					path: []*sdcpb.PathElem{
						{Name: "interface"},
					},
					value: "5",
				},
			},
			want: &JEntry{
				etype: ETMap,
				// name:  "root",
				mapVal: map[string]*JEntry{
					"interface": NewJEntryString("5"),
				},
			},
		},
		{
			name:    "Complex Scenario",
			wantErr: false,
			pvs: []struct {
				path  []*sdcpb.PathElem
				value string
			}{
				{
					path: []*sdcpb.PathElem{
						{Name: "interface", Key: map[string]string{"name": "eth0"}},
						{Name: "subinterface", Key: map[string]string{"name": "999"}},
						{Name: "vlan-id"},
					},
					value: "5",
				},
				{
					path: []*sdcpb.PathElem{
						{Name: "interface", Key: map[string]string{"name": "eth0"}},
						{Name: "subinterface", Key: map[string]string{"name": "996"}},
						{Name: "vlan-id"},
					},
					value: "88",
				},
				{
					path: []*sdcpb.PathElem{
						{Name: "interface", Key: map[string]string{"name": "eth1"}},
						{Name: "subinterface", Key: map[string]string{"name": "76"}},
						{Name: "vlan-id"},
					},
					value: "8",
				},
				{
					path: []*sdcpb.PathElem{
						{Name: "interface", Key: map[string]string{"name": "eth1"}},
						{Name: "mtu"},
					},
					value: "1500",
				},
				{
					path: []*sdcpb.PathElem{
						{Name: "system"},
						{Name: "file", Key: map[string]string{"name": "foo", "bar": "Second Key Val"}},
						{Name: "filename"},
					},
					value: "funnyfile.log",
				},
				{
					path: []*sdcpb.PathElem{
						{Name: "system"},
						{Name: "file", Key: map[string]string{"name": "foo", "bar": "Second Key Val"}},
						{Name: "path"},
					},
					value: "/some/system/path",
				},
			},
			want: &JEntry{
				etype: ETMap,
				mapVal: map[string]*JEntry{
					"interface": {
						etype: ETArray,
						arrayVal: []*JEntry{
							{
								etype: ETMap,
								mapVal: map[string]*JEntry{
									"name": NewJEntryString("eth0"),
									"subinterface": {
										etype: ETArray,
										arrayVal: []*JEntry{
											{
												etype: ETMap,
												mapVal: map[string]*JEntry{
													"name":    NewJEntryString("999"),
													"vlan-id": NewJEntryString("5"),
												},
											},
											{
												etype: ETMap,
												mapVal: map[string]*JEntry{
													"name":    NewJEntryString("996"),
													"vlan-id": NewJEntryString("88"),
												},
											},
										},
									},
								},
							},
							{
								etype: ETMap,
								mapVal: map[string]*JEntry{
									"name": NewJEntryString("eth1"),
									"subinterface": {
										etype: ETArray,
										arrayVal: []*JEntry{
											{
												etype: ETMap,
												mapVal: map[string]*JEntry{
													"name":    NewJEntryString("76"),
													"vlan-id": NewJEntryString("8"),
												},
											},
										},
									},
									"mtu": NewJEntryString("1500"),
								},
							},
						},
					},
					"system": {
						etype: ETMap,
						mapVal: map[string]*JEntry{
							"file": {
								etype: ETArray,
								arrayVal: []*JEntry{
									{
										etype: ETMap,
										mapVal: map[string]*JEntry{
											"name":     NewJEntryString("foo"),
											"bar":      NewJEntryString("Second Key Val"),
											"filename": NewJEntryString("funnyfile.log"),
											"path":     NewJEntryString("/some/system/path"),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jb := NewJsonBuilder()
			for _, pathValueItem := range tt.pvs {
				if err := jb.AddValue(pathValueItem.path, pathValueItem.value); (err != nil) != tt.wantErr {
					t.Errorf("JsonBuilder.AddPath() error = %v, wantErr %v", err, tt.wantErr)
				}
			}

			// PrintDoc(jb)
			// PrintDoc(&JsonBuilder{root: tt.want})

			if !cmp.Equal(jb.root, tt.want, cmp.AllowUnexported(JEntry{}), cmpopts.IgnoreFields(JEntry{}, "name")) {
				d := cmp.Diff(jb.root, tt.want, cmp.AllowUnexported(JEntry{}), cmpopts.IgnoreFields(JEntry{}, "name"))
				t.Error(d)
			}
		})
	}
}

// func PrintDoc(jb *JsonBuilder) {
// 	doc, err := jb.GetDocIndent()
// 	if err != nil {
// 		fmt.Println(err)
// 	}
// 	fmt.Println(string(doc))
// }
