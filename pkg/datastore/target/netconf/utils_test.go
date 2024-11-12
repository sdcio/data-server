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

package netconf

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func Test_pathElem2Xpath(t *testing.T) {
	type args struct {
		pe        *sdcpb.PathElem
		namespace string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "PathElem without keys",
			args: args{
				pe: &sdcpb.PathElem{
					Name: "interfaces",
				},
				namespace: "",
			},
			want:    "./interfaces",
			wantErr: false,
		},
		{
			name: "PathElem with single key",
			args: args{
				pe: &sdcpb.PathElem{
					Name: "interface",
					Key: map[string]string{
						"name": "eth0",
					},
				},
				namespace: "",
			},
			want:    "./interface[name='eth0']",
			wantErr: false,
		},
		{
			name: "PathElem with two keys",
			args: args{
				pe: &sdcpb.PathElem{
					Name: "interface",
					Key: map[string]string{
						"name":  "eth0",
						"state": "up",
					},
				},
				namespace: "",
			},
			want:    "./interface[name='eth0',state='up']",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pathElem2XPath(tt.args.pe)
			if (err != nil) != tt.wantErr {
				t.Errorf("pathElem2Xpath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if d := cmp.Diff(got, tt.want); d != "" {
				t.Errorf("pathElem2XPath() diff:\n%s", d)
			}

		})
	}
}

// func TestXMLConfig_Add(t *testing.T) {
// 	type fields struct {
// 		doc *etree.Document
// 	}
// 	type args struct {
// 		p *sdcpb.Path
// 		v *sdcpb.TypedValue
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		args    args
// 		wantErr bool
// 	}{
// 		{
// 			name: "String value",
// 			fields: fields{
// 				doc: etree.NewDocument(),
// 			},
// 			wantErr: false,
// 			args: args{
// 				p: &sdcpb.Path{
// 					Elem: []*sdcpb.PathElem{
// 						{
// 							Name: "interface",
// 							Key: map[string]string{
// 								"name": "eth0",
// 							},
// 						},
// 						{
// 							Name: "subinterface",
// 							Key: map[string]string{
// 								"id": "0",
// 							},
// 						},
// 						{
// 							Name: "description",
// 						},
// 					},
// 				},
// 				v: &sdcpb.TypedValue{
// 					Value: &sdcpb.TypedValue_StringVal{
// 						StringVal: "MyDesciption",
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "Int value",
// 			fields: fields{
// 				doc: etree.NewDocument(),
// 			},
// 			wantErr: false,
// 			args: args{
// 				p: &sdcpb.Path{
// 					Elem: []*sdcpb.PathElem{
// 						{
// 							Name: "interface",
// 							Key: map[string]string{
// 								"name": "eth0",
// 							},
// 						},
// 						{
// 							Name: "subinterface",
// 							Key: map[string]string{
// 								"id": "0",
// 							},
// 						},
// 						{
// 							Name: "description",
// 						},
// 					},
// 				},
// 				v: &sdcpb.TypedValue{
// 					Value: &sdcpb.TypedValue_IntVal{
// 						IntVal: 35,
// 					},
// 				},
// 			},
// 		},
// 	}
// for _, tt := range tests {
// 	t.Run(tt.name, func(t *testing.T) {
// 		x := &XMLConfigBuilder{
// 			doc: tt.fields.doc,
// 		}
// 		if err := x.Add(tt.args.p, tt.args.v); (err != nil) != tt.wantErr {
// 			t.Errorf("XMLConfig.Add() error = %v, wantErr %v", err, tt.wantErr)
// 		}
// 		x.doc.WriteTo(os.Stdout)
// 	})
// }
//}

func Test_getNamespaceFromGetSchemaResponse(t *testing.T) {
	tests := []struct {
		name string
		sr   *sdcpb.GetSchemaResponse
		want string
	}{

		{
			name: "ContainerSchema",
			sr: &sdcpb.GetSchemaResponse{
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Container{
						Container: &sdcpb.ContainerSchema{
							Namespace: "Container",
						},
					},
				},
			},
			want: "Container",
		},
		{
			name: "FieldSchema",
			sr: &sdcpb.GetSchemaResponse{
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Field{
						Field: &sdcpb.LeafSchema{
							Namespace: "Field",
						},
					},
				},
			},
			want: "Field",
		},
		{
			name: "LeafListSchema",
			sr: &sdcpb.GetSchemaResponse{
				Schema: &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Leaflist{
						Leaflist: &sdcpb.LeafListSchema{
							Namespace: "LeafList",
						},
					},
				},
			},
			want: "LeafList",
		},
		{
			name: "Unknown",
			sr: &sdcpb.GetSchemaResponse{
				Schema: &sdcpb.SchemaElem{
					Schema: nil,
				},
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getNamespaceFromGetSchemaResponse(tt.sr); got != tt.want {
				t.Errorf("getNamespaceFromGetSchemaResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}
