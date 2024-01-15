package netconf

import (
	"context"
	"reflect"
	"testing"

	"github.com/beevik/etree"
	"github.com/iptecharch/data-server/mocks/mockschema"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
)

func TestXML2sdcpbConfigAdapter_Transform(t *testing.T) {

	GetNewDoc := func() *etree.Document {
		AddValueDoc1 := etree.NewDocument()
		// populate Doc1
		interfs := AddValueDoc1.CreateElement("interfaces")
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
		getXML2sdcpbConfigAdapter func(ctrl *gomock.Controller) *XML2sdcpbConfigAdapter
		args                      args
		want                      *sdcpb.Notification
		wantErr                   bool
	}{
		{
			name: "Test One",
			args: args{
				ctx: TestCtx,
				doc: GetNewDoc(),
			},
			getXML2sdcpbConfigAdapter: func(ctrl *gomock.Controller) *XML2sdcpbConfigAdapter {

				schemaClientMock := mockschema.NewMockClient(ctrl)
				counter := 0
				schemaClientMock.EXPECT().GetSchema(TestCtx, gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
					func(ctx context.Context, in *sdcpb.GetSchemaRequest, opts ...grpc.CallOption) (*sdcpb.GetSchemaResponse, error) {
						selem := &sdcpb.SchemaElem{}
						switch counter {
						case 0:
							selem.Schema = &sdcpb.SchemaElem_Container{
								Container: &sdcpb.ContainerSchema{
									Name: "interfaces",
								},
							}
						case 1:
							selem.Schema = &sdcpb.SchemaElem_Container{
								Container: &sdcpb.ContainerSchema{
									Name: "interface",
									Keys: []*sdcpb.LeafSchema{
										{
											Name: "name",
										},
									},
								},
							}
						case 2:
							selem.Schema = &sdcpb.SchemaElem_Field{
								Field: &sdcpb.LeafSchema{
									Name: "name",
									Type: &sdcpb.SchemaLeafType{
										Type: "string",
									},
								},
							}
						}
						counter++

						return &sdcpb.GetSchemaResponse{
							Schema: selem,
						}, nil
					},
				)
				return NewXML2sdcpbConfigAdapter(schemaClientMock, TestSchema)
			},
			want: &sdcpb.Notification{
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
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			x := tt.getXML2sdcpbConfigAdapter(mockCtrl)
			// tt.args.doc.Indent(2)
			// s, _ := tt.args.doc.WriteToString()
			// fmt.Println(s)
			got, err := x.Transform(tt.args.ctx, tt.args.doc)
			if (err != nil) != tt.wantErr {
				t.Errorf("XML2sdcpbConfigAdapter.Transform() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("XML2sdcpbConfigAdapter.Transform() = %v, want %v", got, tt.want)
			}
		})
	}
}