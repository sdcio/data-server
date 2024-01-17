package jsonbuilder

import (
	"fmt"
	"testing"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
)

func TestJsonBuilder_AddPath(t *testing.T) {
	tests := []struct {
		name string
		pvs  []struct {
			path  []*sdcpb.PathElem
			value string
		}
		wantErr bool
	}{
		{
			name:    "One",
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
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jb := NewJsonBuilder()
			for _, pathValueItem := range tt.pvs {
				if err := jb.AddValue(pathValueItem.path, pathValueItem.value); (err != nil) != tt.wantErr {
					PrintDoc(jb)
					t.Errorf("JsonBuilder.AddPath() error = %v, wantErr %v", err, tt.wantErr)
				}
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
