package target

import (
	"context"
	"reflect"
	"testing"

	"github.com/beevik/etree"
	"github.com/iptecharch/data-server/mocks/mocknetconf"
	"github.com/iptecharch/data-server/mocks/mockschema"
	"github.com/iptecharch/data-server/pkg/config"
	"github.com/iptecharch/data-server/pkg/datastore/target/netconf"
	"github.com/iptecharch/data-server/pkg/datastore/target/netconf/types"
	"github.com/iptecharch/data-server/pkg/schema"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
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
					err := responseDoc.ReadFromString("<interface>eth0</interface>")
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
					s.EXPECT().GetSchema(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
						func(ctx context.Context, in *sdcpb.GetSchemaRequest, opts ...grpc.CallOption) (*sdcpb.GetSchemaResponse, error) {
							return &sdcpb.GetSchemaResponse{
								Schema: &sdcpb.SchemaElem{
									Schema: &sdcpb.SchemaElem_Field{
										Field: &sdcpb.LeafSchema{
											Name: "Foo",
											Type: &sdcpb.SchemaLeafType{
												TypeName: "string",
											},
										},
									},
								},
							}, nil
						},
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
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// create Mock controller
			mockCtrl := gomock.NewController(t)

			tr := &ncTarget{
				name:         tt.fields.name,
				driver:       tt.fields.getDriver(mockCtrl, t),
				connected:    tt.fields.connected,
				schemaClient: tt.fields.getSchemaClient(mockCtrl, t),
				schema:       tt.fields.schema,
				sbiConfig:    tt.fields.sbiConfig,
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
