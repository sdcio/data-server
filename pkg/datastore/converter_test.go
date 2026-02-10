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

package datastore

import (
	"context"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	SchemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestDatastore_expandUpdateLeafAsKeys(t *testing.T) {
	type args struct {
		ctx context.Context
		upd *sdcpb.Update
	}
	tests := []struct {
		name    string
		args    args
		want    []*sdcpb.Update
		wantErr bool
	}{
		{
			name: "no_input",
			args: args{
				ctx: context.Background(),
				upd: &sdcpb.Update{},
			},
			want:    []*sdcpb.Update{},
			wantErr: false,
		},
		{
			name: "no_keys",
			args: args{
				ctx: context.Background(),
				upd: &sdcpb.Update{
					Path: &sdcpb.Path{
						Elem: []*sdcpb.PathElem{
							{
								Name: "system",
							},
							{
								Name: "name",
							},
							{
								Name: "host-name",
							},
						},
					},
					Value: &sdcpb.TypedValue{},
				},
			},
			want:    []*sdcpb.Update{},
			wantErr: false,
		},
		{
			name: "single_key_end",
			args: args{
				ctx: context.Background(),
				upd: &sdcpb.Update{
					Path: &sdcpb.Path{
						Elem: []*sdcpb.PathElem{
							{
								Name: "interface",
								Key:  map[string]string{"name": "ethernet-1/1"},
							},
							{
								Name: "admin-state",
							},
						},
					},
					Value: &sdcpb.TypedValue{
						Value: &sdcpb.TypedValue_StringVal{StringVal: "enable"},
					},
				},
			},
			want: []*sdcpb.Update{
				{
					Path: &sdcpb.Path{
						Elem: []*sdcpb.PathElem{
							{
								Name: "interface",
								Key:  map[string]string{"name": "ethernet-1/1"},
							},
							{
								Name: "name",
							},
						},
					},
					Value: &sdcpb.TypedValue{
						Value: &sdcpb.TypedValue_StringVal{StringVal: "ethernet-1/1"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "single_key_middle",
			args: args{
				ctx: context.Background(),
				upd: &sdcpb.Update{
					Path: &sdcpb.Path{
						Elem: []*sdcpb.PathElem{
							{
								Name: "interface",
								Key:  map[string]string{"name": "ethernet-1/1"},
							},
							{
								Name: "ethernet",
							},
							{
								Name: "port-speed",
							},
						},
					},
					Value: &sdcpb.TypedValue{
						Value: &sdcpb.TypedValue_StringVal{StringVal: "400G"},
					},
				},
			},
			want: []*sdcpb.Update{
				{
					Path: &sdcpb.Path{
						Elem: []*sdcpb.PathElem{
							{
								Name: "interface",
								Key:  map[string]string{"name": "ethernet-1/1"},
							},
							{
								Name: "name",
							},
						},
					},
					Value: &sdcpb.TypedValue{
						Value: &sdcpb.TypedValue_StringVal{StringVal: "ethernet-1/1"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple_keys_end",
			args: args{
				ctx: context.Background(),
				upd: &sdcpb.Update{
					Path: &sdcpb.Path{
						Elem: []*sdcpb.PathElem{
							{
								Name: "doublekey",
								Key: map[string]string{
									"key1": "k1foo",
									"key2": "k2123",
								},
							},
							{
								Name: "cont",
							},
							{
								Name: "value1",
							},
						},
					},
				},
			},
			want: []*sdcpb.Update{
				{
					Path: &sdcpb.Path{
						Elem: []*sdcpb.PathElem{
							{
								Name: "doublekey",
								Key: map[string]string{
									"key1": "k1foo",
									"key2": "k2123",
								},
							},
							{
								Name: "key1",
							},
						},
					},
					Value: &sdcpb.TypedValue{
						Value: &sdcpb.TypedValue_StringVal{StringVal: "k1foo"},
					},
				},
				{
					Path: &sdcpb.Path{
						Elem: []*sdcpb.PathElem{
							{
								Name: "doublekey",
								Key: map[string]string{
									"key1": "k1foo",
									"key2": "k2123",
								},
							},
							{
								Name: "key2",
							},
						},
					},
					Value: &sdcpb.TypedValue{
						Value: &sdcpb.TypedValue_StringVal{StringVal: "k2123"},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			schemaClient, schema, err := testhelper.InitSDCIOSchema()
			if err != nil {
				t.Error(err)
				return
			}

			scb := SchemaClient.NewSchemaClientBound(schema, schemaClient)
			converter := utils.NewConverter(scb)

			got, err := converter.ExpandUpdateKeysAsLeaf(tt.args.ctx, tt.args.upd)
			if (err != nil) != tt.wantErr {
				t.Errorf("Datastore.expandUpdateLeafAsKeys() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// convert got to string array
			gotStrArr := []string{}
			for _, x := range got {
				gotStrArr = append(gotStrArr, x.String())
			}
			slices.Sort(gotStrArr)

			// convert want to string array
			wantStrArr := []string{}
			for _, x := range tt.want {
				wantStrArr = append(wantStrArr, x.String())
			}
			slices.Sort(wantStrArr)

			// compare the string arrays
			if diff := cmp.Diff(wantStrArr, gotStrArr); diff != "" {
				t.Errorf("mismatch (-want +got)\n%s", diff)
				return
			}
		})
	}
}
