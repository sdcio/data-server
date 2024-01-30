package jbuilderv2

import (
	"reflect"
	"testing"
)

func Test_jsonBuilder_addValueToObject(t *testing.T) {
	type args struct {
		obj   map[string]any
		path  []*pathElem
		value interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]any
		wantErr bool
	}{
		{
			name: "add_path_no_lists",
			args: args{
				obj: map[string]any{},
				path: []*pathElem{
					{
						name: "system",
					},
					{
						name: "name",
					},
					{
						name: "host-name",
					},
				},
				value: "SRLinux1",
			},
			want: map[string]any{
				"system": map[string]any{
					"name": map[string]any{
						"host-name": "SRLinux1",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "add_path_to_new_list",
			args: args{
				obj: map[string]any{},
				path: []*pathElem{
					{
						name: "interface",
						keyValueType: map[string]*vt{
							"name": {
								value:  "ethernet-1/1",
								conVal: "ethernet-1/1",
							},
						},
					},
					{
						name: "subinterface",
						keyValueType: map[string]*vt{
							"index": {
								value:  "0",
								conVal: int(0),
							},
						},
					},
					{
						name: "admin-state",
					},
				},
				value: "enable",
			},
			want: map[string]any{
				"interface": []map[string]any{
					{
						"name": "ethernet-1/1",
						"subinterface": []map[string]any{
							{"index": int(0), "admin-state": "enable"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "add_path_to_existing_list",
			args: args{
				obj: map[string]any{
					"interface": []map[string]any{
						{
							"name": "ethernet-1/1",
							"subinterface": []map[string]any{
								{"index": int(0), "admin-state": "enable"},
							},
						},
					},
				},
				path: []*pathElem{
					{
						name: "interface",
						keyValueType: map[string]*vt{
							"name": {
								value:  "ethernet-1/1",
								conVal: "ethernet-1/1",
							},
						},
					},
					{
						name: "subinterface",
						keyValueType: map[string]*vt{
							"index": {
								value:  "1",
								conVal: int(1),
							},
						},
					},
					{
						name: "admin-state",
					},
				},
				value: "enable",
			},
			want: map[string]any{
				"interface": []map[string]any{
					{
						"name": "ethernet-1/1",
						"subinterface": []map[string]any{
							{"index": int(0), "admin-state": "enable"},
							{"index": int(1), "admin-state": "enable"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "add_path_to_leaf-list",
			args: args{
				obj: map[string]any{},
				path: []*pathElem{
					{
						name: "system",
					},
					{
						name: "gnmi-server",
					},
					{
						name: "network-instance",
						keyValueType: map[string]*vt{
							"name": {
								value:  "mgmt",
								conVal: "mgmt",
							},
						},
					},
					{
						name: "services",
					},
				},
				value: []any{"gnmi"},
			},
			want: map[string]any{
				"system": map[string]any{
					"gnmi-server": map[string]any{
						"network-instance": []map[string]any{
							{
								"name": "mgmt",
								"services": []any{
									"gnmi",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "add_path_to_existing_leaf-list",
			args: args{
				obj: map[string]any{
					"system": map[string]any{
						"gnmi-server": map[string]any{
							"network-instance": []map[string]any{
								{
									"name": "mgmt",
									"services": []any{
										"gnmi",
									},
								},
							},
						},
					},
				},
				path: []*pathElem{
					{
						name: "system",
					},
					{
						name: "gnmi-server",
					},
					{
						name: "network-instance",
						keyValueType: map[string]*vt{
							"name": {
								value:  "mgmt",
								conVal: "mgmt",
							},
						},
					},
					{
						name: "services",
					},
				},
				value: []any{"gnoi"},
			},
			want: map[string]any{
				"system": map[string]any{
					"gnmi-server": map[string]any{
						"network-instance": []map[string]any{
							{
								"name": "mgmt",
								"services": []any{
									"gnmi",
									"gnoi",
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
			j := &jsonBuilder{}
			if err := j.addValueToObject(tt.args.obj, tt.args.path, tt.args.value); (err != nil) != tt.wantErr {
				t.Errorf("jsonBuilder.addValueToObject() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.args.obj, tt.want) {
				t.Errorf("failed: got : %v", tt.args.obj)
				t.Errorf("failed: want: %v", tt.want)
			}
		})
	}
}
