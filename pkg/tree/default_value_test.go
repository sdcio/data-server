package tree

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestDefaultValueExists(t *testing.T) {
	tests := []struct {
		name       string
		schemaElem func(t *testing.T) *sdcpb.SchemaElem
		want       bool
	}{
		{
			name: "no default",
			schemaElem: func(t *testing.T) *sdcpb.SchemaElem {
				sc, schema, err := testhelper.InitSDCIOSchema()
				if err != nil {
					t.Fatal(err)
				}

				ctx := context.Background()
				scb := schemaClient.NewSchemaClientBound(schema, sc)

				rsp, err := scb.GetSchemaSlicePath(ctx, []string{})
				if err != nil {
					t.Fatal(err)
				}
				return rsp.GetSchema()
			},
			want: false,
		},
		{
			name: "field default",
			schemaElem: func(t *testing.T) *sdcpb.SchemaElem {
				sc, schema, err := testhelper.InitSDCIOSchema()
				if err != nil {
					t.Fatal(err)
				}

				ctx := context.Background()
				scb := schemaClient.NewSchemaClientBound(schema, sc)

				rsp, err := scb.GetSchemaSlicePath(ctx, []string{"choices", "case1", "log"})
				if err != nil {
					t.Fatal(err)
				}
				return rsp.GetSchema()
			},
			want: true,
		},
		{
			name: "leaflist default",
			schemaElem: func(t *testing.T) *sdcpb.SchemaElem {
				sc, schema, err := testhelper.InitSDCIOSchema()
				if err != nil {
					t.Fatal(err)
				}

				ctx := context.Background()
				scb := schemaClient.NewSchemaClientBound(schema, sc)

				rsp, err := scb.GetSchemaSlicePath(ctx, []string{"leaflist", "with-default"})
				if err != nil {
					t.Fatal(err)
				}
				return rsp.GetSchema()
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultValueExists(tt.schemaElem(t)); got != tt.want {
				t.Errorf("DefaultValueExists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultValueRetrieve(t *testing.T) {
	tests := []struct {
		name       string
		schemaElem func(t *testing.T) *sdcpb.SchemaElem
		wanterr    bool
		want       *types.Update
	}{
		{
			name: "no default",
			schemaElem: func(t *testing.T) *sdcpb.SchemaElem {
				sc, schema, err := testhelper.InitSDCIOSchema()
				if err != nil {
					t.Fatal(err)
				}

				ctx := context.Background()
				scb := schemaClient.NewSchemaClientBound(schema, sc)

				rsp, err := scb.GetSchemaSlicePath(ctx, []string{})
				if err != nil {
					t.Fatal(err)
				}
				return rsp.GetSchema()
			},
			wanterr: true,
			want:    nil,
		},
		{
			name: "field default",
			schemaElem: func(t *testing.T) *sdcpb.SchemaElem {
				sc, schema, err := testhelper.InitSDCIOSchema()
				if err != nil {
					t.Fatal(err)
				}

				ctx := context.Background()
				scb := schemaClient.NewSchemaClientBound(schema, sc)

				rsp, err := scb.GetSchemaSlicePath(ctx, []string{"choices", "case1", "log"})
				if err != nil {
					t.Fatal(err)
				}
				return rsp.GetSchema()
			},
			wanterr: false,
			want:    types.NewUpdate(types.PathSlice{}, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: false}}, 5, "owner1", 0),
		},
		{
			name: "leaflist default",
			schemaElem: func(t *testing.T) *sdcpb.SchemaElem {
				sc, schema, err := testhelper.InitSDCIOSchema()
				if err != nil {
					t.Fatal(err)
				}

				ctx := context.Background()
				scb := schemaClient.NewSchemaClientBound(schema, sc)

				rsp, err := scb.GetSchemaSlicePath(ctx, []string{"leaflist", "with-default"})
				if err != nil {
					t.Fatal(err)
				}
				return rsp.GetSchema()
			},
			wanterr: false,
			want:    types.NewUpdate(types.PathSlice{}, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_LeaflistVal{LeaflistVal: &sdcpb.ScalarArray{Element: []*sdcpb.TypedValue{{Value: &sdcpb.TypedValue_StringVal{StringVal: "foo"}}, {Value: &sdcpb.TypedValue_StringVal{StringVal: "bar"}}}}}}, 5, "owner1", 0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := DefaultValueRetrieve(tt.schemaElem(t), []string{}, 5, "owner1")
			if tt.wanterr {
				if err == nil {
					t.Fatalf("expected err, got non")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(tt.want.String(), val.String()); diff != "" {
				t.Fatalf("mismatching defaults (-want +got)\n%s", diff)
			}
		})
	}
}
