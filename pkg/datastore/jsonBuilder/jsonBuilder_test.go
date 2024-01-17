package jsonbuilder

import (
	"fmt"
	"testing"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
)

func TestJsonBuilder_AddPath(t *testing.T) {
	tests := []struct {
		name    string
		path    []*sdcpb.PathElem
		n       *sdcpb.Notification
		value   string
		wantErr bool
	}{
		{
			name:    "One",
			wantErr: false,
			path: []*sdcpb.PathElem{
				{Name: "interface", Key: map[string]string{"name": "eth0"}},
				{Name: "subinterface", Key: map[string]string{"name": "999"}},
				{Name: "vlan-id"},
			},
			value: "5",
			n: &sdcpb.Notification{
				Update: []*sdcpb.Update{
					{
						Path: &sdcpb.Path{
							Elem: []*sdcpb.PathElem{},
						},
						Value: &sdcpb.TypedValue{
							Value: &sdcpb.TypedValue_StringVal{},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jb := NewJsonBuilder()
			if err := jb.AddValue(tt.path, tt.value); (err != nil) != tt.wantErr {
				PrintDoc(jb)
				t.Errorf("JsonBuilder.AddPath() error = %v, wantErr %v", err, tt.wantErr)
			}
			PrintDoc(jb)
		})
	}
}

func PrintDoc(jb *JsonBuilder) {
	doc, err := jb.GetDocIndent()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(string(doc))
}
