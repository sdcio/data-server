package json

import (
	"context"
	"reflect"
	"testing"

	"github.com/sdcio/data-server/pkg/tree/importer"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestJsonTreeImporter_GetElement(t *testing.T) {
	type fields struct {
		data any
	}
	type args struct {
		key string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   importer.ImportConfigAdapter
	}{
		{
			name: "one",
			args: args{
				key: "foo",
			},
			fields: fields{
				data: map[string]any{
					"foo": "bar",
				},
			},
			want: &JsonTreeImporter{
				data: "bar",
				name: "foo",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := NewJsonTreeImporter(tt.fields.data)
			if got := j.GetElement(tt.args.key); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("JsonTreeImporter.GetElement() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJsonTreeImporter_GetKeyValue(t *testing.T) {
	type fields struct {
		data any
		name string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "string",
			fields: fields{
				data: "bar",
				name: "foo",
			},
			want: "bar",
		},
		{
			name: "int",
			fields: fields{
				data: 5,
				name: "bar",
			},
			want: "5",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := newJsonTreeImporterInternal(tt.fields.name, tt.fields.data)

			if got, _ := j.GetKeyValue(); got != tt.want {
				t.Errorf("JsonTreeImporter.GetKeyValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJsonTreeImporter_GetName(t *testing.T) {
	type fields struct {
		data any
		name string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "one",
			fields: fields{
				name: "foo",
			},
			want: "foo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := &JsonTreeImporter{
				data: tt.fields.data,
				name: tt.fields.name,
			}
			if got := j.GetName(); got != tt.want {
				t.Errorf("JsonTreeImporter.GetName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJsonTreeImporter_GetTVValue(t *testing.T) {
	ctx := context.TODO()
	type fields struct {
		data any
		name string
	}
	type args struct {
		slt *sdcpb.SchemaLeafType
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *sdcpb.TypedValue
		wantErr bool
	}{
		{
			name: "string",
			fields: fields{
				name: "foo",
				data: "foobar",
			},
			args: args{
				&sdcpb.SchemaLeafType{
					Type: "string",
				},
			},
			wantErr: false,
			want:    &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "foobar"}},
		},
		{
			name: "int",
			fields: fields{
				name: "foo",
				data: int32(5),
			},
			args: args{
				&sdcpb.SchemaLeafType{
					Type: "int32",
				},
			},
			wantErr: false,
			want:    &sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 5}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := &JsonTreeImporter{
				data: tt.fields.data,
				name: tt.fields.name,
			}
			got, err := j.GetTVValue(ctx, tt.args.slt)
			if (err != nil) != tt.wantErr {
				t.Errorf("JsonTreeImporter.GetTVValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("JsonTreeImporter.GetTVValue() = %v, want %v", got, tt.want)
			}
		})
	}
}
