package ops_test

import (
	"testing"

	mockTreeEntry "github.com/sdcio/data-server/mocks/mocktreeentry"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// alwaysContains is a SensitivePathChecker that treats every path as sensitive.
type alwaysContains struct{}

func (alwaysContains) Contains(*sdcpb.Path) bool { return true }

func TestShouldRedact(t *testing.T) {
	somePath := &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "x"}}}

	sensitiveLeaf := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{Field: &sdcpb.LeafSchema{Sensitive: true}},
	}
	plainLeaf := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{Field: &sdcpb.LeafSchema{Sensitive: false}},
	}

	tests := []struct {
		name             string
		schema           *sdcpb.SchemaElem
		includeSensitive bool
		sps              types.SensitivePathChecker
		want             bool
	}{
		{
			name:   "schema-sensitive leaf, IncludeSensitive=false → redact",
			schema: sensitiveLeaf,
			want:   true,
		},
		{
			name:             "schema-sensitive leaf, IncludeSensitive=true → reveal",
			schema:           sensitiveLeaf,
			includeSensitive: true,
			want:             false,
		},
		{
			name:   "non-sensitive leaf → pass through",
			schema: plainLeaf,
			want:   false,
		},
		{
			name: "sensitive leaf-list → redact",
			schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Leaflist{Leaflist: &sdcpb.LeafListSchema{Sensitive: true}},
			},
			want: true,
		},
		{
			name: "container → pass through",
			schema: &sdcpb.SchemaElem{
				Schema: &sdcpb.SchemaElem_Container{Container: &sdcpb.ContainerSchema{}},
			},
			want: false,
		},
		{
			name:   "nil schema → pass through",
			schema: nil,
			want:   false,
		},
		{
			name:   "path in SensitivePathSet → redact",
			schema: plainLeaf,
			sps:    alwaysContains{},
			want:   true,
		},
		{
			name:             "path in SensitivePathSet, IncludeSensitive=true → reveal",
			schema:           plainLeaf,
			sps:              alwaysContains{},
			includeSensitive: true,
			want:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			e := mockTreeEntry.NewMockEntry(ctrl)
			e.EXPECT().GetSchema().Return(tt.schema)
			e.EXPECT().SdcpbPath().Return(somePath)

			got := ops.ShouldRedact(e, tt.includeSensitive, tt.sps)
			if got != tt.want {
				t.Errorf("ShouldRedact() = %v, want %v", got, tt.want)
			}
		})
	}
}
