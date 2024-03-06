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

package target

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/mocks/mocknetconf"
	"github.com/sdcio/data-server/mocks/mockschema"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore/target/netconf"
	"github.com/sdcio/data-server/pkg/datastore/target/netconf/types"
	"github.com/sdcio/data-server/pkg/schema"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
)

const (
	SchemaName    = "TestModel"
	SchemaVendor  = "TestVendor"
	SchemaVersion = "TestVersion"
)

var (
	TestSchema = &sdcpb.Schema{
		Name:    SchemaName,
		Vendor:  SchemaVendor,
		Version: SchemaVersion,
	}
	TestCtx = context.TODO()
)

func Test_ncTarget_Get(t *testing.T) {
	type fields struct {
		name            string
		getDriver       func(*gomock.Controller, *testing.T) netconf.Driver
		connected       bool
		getSchemaClient func(*gomock.Controller, *testing.T) schema.Client
		schema          *sdcpb.Schema
		sbiConfig       *config.SBI
	}
	type args struct {
		ctx context.Context
		req *sdcpb.GetDataRequest
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *sdcpb.GetDataResponse
		wantErr bool
	}{
		{
			name: "One",
			fields: fields{
				getDriver: func(c *gomock.Controller, t *testing.T) netconf.Driver {
					d := mocknetconf.NewMockDriver(c)
					filter := `<interface>
  <name>eth0</name>
</interface>
`
					responseDoc := etree.NewDocument()
					err := responseDoc.ReadFromString("<interface><name>eth0</name><mtu>1500</mtu></interface>")
					if err != nil {
						t.Errorf("error creating response")
					}
					d.EXPECT().GetConfig("running", filter).Return(&types.NetconfResponse{Doc: responseDoc}, nil)
					return d
				},
				name:      "TestDev",
				connected: true,
				schema:    &sdcpb.Schema{},
				sbiConfig: &config.SBI{
					IncludeNS:              false,
					OperationWithNamespace: false,
					UseOperationRemove:     false,
				},
				getSchemaClient: func(c *gomock.Controller, t *testing.T) schema.Client {
					s := mockschema.NewMockClient(c)
					gomock.InOrder(
						s.EXPECT().GetSchema(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
							func(ctx context.Context, in *sdcpb.GetSchemaRequest, opts ...grpc.CallOption) (*sdcpb.GetSchemaResponse, error) {
								fmt.Println(in.String())
								return &sdcpb.GetSchemaResponse{
									Schema: &sdcpb.SchemaElem{
										Schema: &sdcpb.SchemaElem_Container{
											Container: &sdcpb.ContainerSchema{
												Name: "interface",
												Keys: []*sdcpb.LeafSchema{
													{
														Name: "name",
													},
												},
											},
										},
									},
								}, nil
							},
						),
						s.EXPECT().GetSchema(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
							func(ctx context.Context, in *sdcpb.GetSchemaRequest, opts ...grpc.CallOption) (*sdcpb.GetSchemaResponse, error) {
								fmt.Println(in.String())
								return &sdcpb.GetSchemaResponse{
									Schema: &sdcpb.SchemaElem{
										Schema: &sdcpb.SchemaElem_Container{
											Container: &sdcpb.ContainerSchema{
												Name: "interface",
												Keys: []*sdcpb.LeafSchema{
													{
														Name: "name",
													},
												},
											},
										},
									},
								}, nil
							},
						),
						s.EXPECT().GetSchema(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
							func(ctx context.Context, in *sdcpb.GetSchemaRequest, opts ...grpc.CallOption) (*sdcpb.GetSchemaResponse, error) {
								fmt.Println(in.String())
								return &sdcpb.GetSchemaResponse{
									Schema: &sdcpb.SchemaElem{
										Schema: &sdcpb.SchemaElem_Field{
											Field: &sdcpb.LeafSchema{
												Name: "name",
												Type: &sdcpb.SchemaLeafType{
													Type: "string",
												},
											},
										},
									},
								}, nil
							},
						),
						s.EXPECT().GetSchema(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
							func(ctx context.Context, in *sdcpb.GetSchemaRequest, opts ...grpc.CallOption) (*sdcpb.GetSchemaResponse, error) {
								fmt.Println(in.String())
								return &sdcpb.GetSchemaResponse{
									Schema: &sdcpb.SchemaElem{
										Schema: &sdcpb.SchemaElem_Field{
											Field: &sdcpb.LeafSchema{
												Name: "mtu",
												Type: &sdcpb.SchemaLeafType{

													Type: "string",
												},
											},
										},
									},
								}, nil
							},
						),
					)
					return s
				},
			},
			args: args{
				ctx: context.Background(),
				req: &sdcpb.GetDataRequest{
					Datastore: &sdcpb.DataStore{
						Type: sdcpb.Type_MAIN,
					},
					Path: []*sdcpb.Path{
						{
							Elem: []*sdcpb.PathElem{
								{
									Name: "interface",
									Key: map[string]string{
										"name": "eth0",
									},
								},
							},
						},
					},
				},
			},
			want: &sdcpb.GetDataResponse{
				Notification: []*sdcpb.Notification{
					{
						Update: []*sdcpb.Update{
							{
								Path: &sdcpb.Path{
									Elem: []*sdcpb.PathElem{
										{
											Name: "interface",
											Key: map[string]string{
												"name": "eth0",
											},
										},
										{
											Name: "name",
										},
									},
								},
								Value: &sdcpb.TypedValue{
									Value: &sdcpb.TypedValue_StringVal{
										StringVal: "eth0",
									},
								},
							},
							{
								Path: &sdcpb.Path{
									Elem: []*sdcpb.PathElem{
										{
											Name: "interface",
											Key: map[string]string{
												"name": "eth0",
											},
										},
										{
											Name: "mtu",
										},
									},
								},
								Value: &sdcpb.TypedValue{
									Value: &sdcpb.TypedValue_StringVal{
										StringVal: "1500",
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
			// create Mock controller
			mockCtrl := gomock.NewController(t)

			sc := tt.fields.getSchemaClient(mockCtrl, t)

			tr := &ncTarget{
				name:             tt.fields.name,
				driver:           tt.fields.getDriver(mockCtrl, t),
				connected:        tt.fields.connected,
				schemaClient:     sc,
				schema:           tt.fields.schema,
				sbiConfig:        tt.fields.sbiConfig,
				xml2sdcpbAdapter: netconf.NewXML2sdcpbConfigAdapter(sc, tt.fields.schema),
			}
			got, err := tr.Get(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ncTarget.Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ncTarget.Get() = %v, want %v", got, tt.want)
			}
			mockCtrl.Finish()
		})
	}
}

func Test_stripDataContainerRoot(t *testing.T) {
	type args struct {
		n *sdcpb.Notification
	}
	tests := []struct {
		name string
		args args
		want *sdcpb.Notification
	}{
		{
			name: "test0",
			args: args{
				n: &sdcpb.Notification{
					Timestamp: 42,
					Update: []*sdcpb.Update{{
						Path:  &sdcpb.Path{},
						Value: &sdcpb.TypedValue{},
					}},
				},
			},
			want: &sdcpb.Notification{
				Timestamp: 42,
				Update: []*sdcpb.Update{{
					Path:  &sdcpb.Path{},
					Value: &sdcpb.TypedValue{},
				}},
			},
		},
		{
			name: "test1",
			args: args{
				n: &sdcpb.Notification{
					Timestamp: 42,
					Update: []*sdcpb.Update{{
						Path: &sdcpb.Path{
							Elem: []*sdcpb.PathElem{
								{
									Name: "data",
								},
								{
									Name: "configure"},
							},
						},
						Value: &sdcpb.TypedValue{},
					}},
				},
			},
			want: &sdcpb.Notification{
				Timestamp: 42,
				Update: []*sdcpb.Update{{
					Path: &sdcpb.Path{
						Elem: []*sdcpb.PathElem{
							{
								Name: "configure",
							},
						},
					},
					Value: &sdcpb.TypedValue{},
				}},
			},
		},
		{
			name: "test2",
			args: args{
				n: &sdcpb.Notification{
					Timestamp: 42,
					Update: []*sdcpb.Update{{
						Path: &sdcpb.Path{
							Elem: []*sdcpb.PathElem{
								{
									Name: "not_data",
								},
								{
									Name: "configure",
								},
							},
						},
						Value: &sdcpb.TypedValue{},
					}},
				},
			},
			want: &sdcpb.Notification{
				Timestamp: 42,
				Update: []*sdcpb.Update{{
					Path: &sdcpb.Path{
						Elem: []*sdcpb.PathElem{
							{
								Name: "not_data",
							},
							{
								Name: "configure",
							},
						},
					},
					Value: &sdcpb.TypedValue{},
				}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripRootDataContainer(tt.args.n); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("stripDataContainerRoot() = %v, want %v", got, tt.want)
			}
		})
	}
}
