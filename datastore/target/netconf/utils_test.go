package netconf

import (
	"reflect"
	"testing"

	"github.com/beevik/etree"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

func Test_pathElem2Xpath(t *testing.T) {
	type args struct {
		pe        *schemapb.PathElem
		namespace string
	}
	tests := []struct {
		name    string
		args    args
		want    etree.Path
		wantErr bool
	}{
		{
			name: "one",
			args: args{
				pe: &schemapb.PathElem{
					Name: "interface",
					Key: map[string]string{
						"name": "eth0",
					},
				},
				namespace: "",
			},
			want:    etree.MustCompilePath("./interface[name=eth0]"),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pathElem2Xpath(tt.args.pe, tt.args.namespace)
			if (err != nil) != tt.wantErr {
				t.Errorf("pathElem2Xpath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("pathElem2Xpath() = %v, want %v", got, tt.want)
			}
		})
	}
}

// func TestXMLConfig_Add(t *testing.T) {
// 	type fields struct {
// 		doc *etree.Document
// 	}
// 	type args struct {
// 		p *schemapb.Path
// 		v *schemapb.TypedValue
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
// 				p: &schemapb.Path{
// 					Elem: []*schemapb.PathElem{
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
// 				v: &schemapb.TypedValue{
// 					Value: &schemapb.TypedValue_StringVal{
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
// 				p: &schemapb.Path{
// 					Elem: []*schemapb.PathElem{
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
// 				v: &schemapb.TypedValue{
// 					Value: &schemapb.TypedValue_IntVal{
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
