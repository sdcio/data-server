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
	"context"
	"testing"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/mocks/mockschemaclientbound"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

func TestXML2sdcpbConfigAdapter_Transform(t *testing.T) {

	GetNewDoc := func() *etree.Document {
		AddValueDoc1 := etree.NewDocument()
		dataContainer := AddValueDoc1.CreateElement("data")
		// populate Doc1
		interfs := dataContainer.CreateElement("interfaces")
		interf1 := interfs.CreateElement("interface")
		interfname := interf1.CreateElement("name")
		interfname.SetText("eth0")
		// _ = interf1.CreateElement("mtu")
		//subinterf11 := interf1.CreateElement("subinterface")
		//subinterf11name := subinterf11.CreateElement("name")
		//subinterf11name.SetText("1")
		// _ = subinterf11.CreateElement("vlan-id")
		// interf2 := interfs.CreateElement("interface")
		// interf2name := interf2.CreateElement("name")
		// interf2name.SetText("eth1")
		// mtuInterf2 := interf2.CreateElement("mtu")
		// mtuInterf2.SetText("9100")
		return AddValueDoc1
		// Doc1 done
	}

	type args struct {
		ctx context.Context
		doc *etree.Document
	}
	tests := []struct {
		name                      string
		getXML2sdcpbConfigAdapter func(ctrl *gomock.Controller, t *testing.T) *XML2sdcpbConfigAdapter
		args                      args
		want                      []*sdcpb.Notification
		wantErr                   bool
	}{
		{
			name: "Test One",
			args: args{
				ctx: context.TODO(),
				doc: GetNewDoc(),
			},
			getXML2sdcpbConfigAdapter: func(ctrl *gomock.Controller, t *testing.T) *XML2sdcpbConfigAdapter {

				var expectedPath string

				schemaClientMock := mockschemaclientbound.NewMockSchemaClientBound(ctrl)
				counter := 0
				schemaClientMock.EXPECT().GetSchemaSdcpbPath(context.TODO(), gomock.Any()).AnyTimes().DoAndReturn(
					func(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
						selem := &sdcpb.SchemaElem{}
						switch counter {
						case 0:
							selem.Schema = &sdcpb.SchemaElem_Container{
								Container: &sdcpb.ContainerSchema{
									Name: "interfaces",
								},
							}
							expectedPath = "interfaces"
						case 1:
							selem.Schema = &sdcpb.SchemaElem_Container{
								Container: &sdcpb.ContainerSchema{
									Name: "interface",
									Keys: []*sdcpb.LeafSchema{
										{
											Name: "name",
											Type: &sdcpb.SchemaLeafType{
												Type: "string",
											},
										},
									},
								},
							}
							expectedPath = "interfaces/interface"
						case 2:
							selem.Schema = &sdcpb.SchemaElem_Field{
								Field: &sdcpb.LeafSchema{
									Name: "name",
									Type: &sdcpb.SchemaLeafType{
										Type: "string",
									},
								},
							}
							expectedPath = "interfaces/interface[name=eth0]/name"
						}
						// check for the right input
						if rp := utils.ToXPath(path, false); rp != expectedPath {
							t.Errorf("getSchema expected path %s but got %s", expectedPath, rp)
						}

						counter++
						return &sdcpb.GetSchemaResponse{
							Schema: selem,
						}, nil
					},
				)
				return NewXML2sdcpbConfigAdapter(schemaClientMock)
			},
			want: []*sdcpb.Notification{
				{
					Update: []*sdcpb.Update{
						{
							Path: &sdcpb.Path{
								Elem: []*sdcpb.PathElem{
									{
										Name: "interfaces",
									},
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
					},
				},
			},
			wantErr: false,
		},
		{
			name: "leaflist string",
			args: args{
				ctx: context.TODO(),
				doc: func() *etree.Document {
					doc := etree.NewDocument()
					data := doc.CreateElement("data")
					container := data.CreateElement("leaf-list-container")
					container.CreateElement("item").SetText("item-0")
					container.CreateElement("item").SetText("item-1")
					return doc
				}(),
			},
			getXML2sdcpbConfigAdapter: func(ctrl *gomock.Controller, t *testing.T) *XML2sdcpbConfigAdapter {

				var expectedPath string

				schemaClientMock := mockschemaclientbound.NewMockSchemaClientBound(ctrl)
				counter := 0
				schemaClientMock.EXPECT().GetSchemaSdcpbPath(context.TODO(), gomock.Any()).AnyTimes().DoAndReturn(
					func(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
						lls := &sdcpb.LeafListSchema{
							Name: "item",
							Type: &sdcpb.SchemaLeafType{
								Type: "string",
							},
						}
						selem := &sdcpb.SchemaElem{}
						switch counter {
						case 0:
							selem.Schema = &sdcpb.SchemaElem_Container{
								Container: &sdcpb.ContainerSchema{
									Name: "leaf-list-container",
									Leaflists: []*sdcpb.LeafListSchema{
										lls,
									},
								},
							}
							expectedPath = "leaf-list-container"
						case 1, 2:
							selem.Schema = &sdcpb.SchemaElem_Leaflist{
								Leaflist: lls,
							}
							expectedPath = "leaf-list-container/item"
						}
						// check for the right input
						if rp := utils.ToXPath(path, false); rp != expectedPath {
							t.Errorf("getSchema expected path %s but got %s", expectedPath, rp)
						}

						counter++
						return &sdcpb.GetSchemaResponse{
							Schema: selem,
						}, nil
					},
				)
				return NewXML2sdcpbConfigAdapter(schemaClientMock)
			},
			want: []*sdcpb.Notification{
				{
					Update: []*sdcpb.Update{
						{
							Path: &sdcpb.Path{
								Elem: []*sdcpb.PathElem{
									{
										Name: "leaf-list-container",
									},
									{
										Name: "item",
									},
								},
							},
							Value: &sdcpb.TypedValue{
								Value: &sdcpb.TypedValue_LeaflistVal{
									LeaflistVal: &sdcpb.ScalarArray{
										Element: []*sdcpb.TypedValue{
											{
												Value: &sdcpb.TypedValue_StringVal{
													StringVal: "item-0",
												},
											},
											{
												Value: &sdcpb.TypedValue_StringVal{
													StringVal: "item-1",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "leaflist int32",
			args: args{
				ctx: context.TODO(),
				doc: func() *etree.Document {
					doc := etree.NewDocument()
					data := doc.CreateElement("data")
					container := data.CreateElement("leaf-list-container")
					container.CreateElement("item").SetText("0")
					container.CreateElement("item").SetText("1")
					return doc
				}(),
			},
			getXML2sdcpbConfigAdapter: func(ctrl *gomock.Controller, t *testing.T) *XML2sdcpbConfigAdapter {

				var expectedPath string

				schemaClientMock := mockschemaclientbound.NewMockSchemaClientBound(ctrl)
				counter := 0
				schemaClientMock.EXPECT().GetSchemaSdcpbPath(context.TODO(), gomock.Any()).AnyTimes().DoAndReturn(
					func(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
						lls := &sdcpb.LeafListSchema{
							Name: "item",
							Type: &sdcpb.SchemaLeafType{
								Type: "int32",
							},
						}
						selem := &sdcpb.SchemaElem{}
						switch counter {
						case 0:
							selem.Schema = &sdcpb.SchemaElem_Container{
								Container: &sdcpb.ContainerSchema{
									Name: "leaf-list-container",
									Leaflists: []*sdcpb.LeafListSchema{
										lls,
									},
								},
							}
							expectedPath = "leaf-list-container"
						case 1, 2:
							selem.Schema = &sdcpb.SchemaElem_Leaflist{
								Leaflist: lls,
							}
							expectedPath = "leaf-list-container/item"
						}
						// check for the right input
						if rp := utils.ToXPath(path, false); rp != expectedPath {
							t.Errorf("getSchema expected path %s but got %s", expectedPath, rp)
						}

						counter++
						return &sdcpb.GetSchemaResponse{
							Schema: selem,
						}, nil
					},
				)
				return NewXML2sdcpbConfigAdapter(schemaClientMock)
			},
			want: []*sdcpb.Notification{
				{
					Update: []*sdcpb.Update{
						{
							Path: &sdcpb.Path{
								Elem: []*sdcpb.PathElem{
									{
										Name: "leaf-list-container",
									},
									{
										Name: "item",
									},
								},
							},
							Value: &sdcpb.TypedValue{
								Value: &sdcpb.TypedValue_LeaflistVal{
									LeaflistVal: &sdcpb.ScalarArray{
										Element: []*sdcpb.TypedValue{
											{
												Value: &sdcpb.TypedValue_IntVal{
													IntVal: 0,
												},
											},
											{
												Value: &sdcpb.TypedValue_IntVal{
													IntVal: 1,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			x := tt.getXML2sdcpbConfigAdapter(mockCtrl, t)
			got, err := x.Transform(tt.args.ctx, tt.args.doc)
			if (err != nil) != tt.wantErr {
				t.Errorf("XML2sdcpbConfigAdapter.Transform() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("XML2sdcpbConfigAdapter.Transform() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if !utils.NotificationsEqual(got[i], tt.want[i]) {
					t.Errorf("XML2sdcpbConfigAdapter.Transform() = %v, want %v", got[i], tt.want[i])
				}
			}
		})
	}
}
