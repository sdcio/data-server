package ops_test

import (
	"testing"

	mockTreeEntry "github.com/sdcio/data-server/mocks/mocktreeentry"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

func TestSensitiveRender_TypedValue(t *testing.T) {
	sensitiveLeaf := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{Field: &sdcpb.LeafSchema{Sensitive: true}},
	}
	plainLeaf := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{Field: &sdcpb.LeafSchema{Sensitive: false}},
	}
	real := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "secret"}}
	path := &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "secret"}}}
	marker := types.NewSensitivePaths(path)

	tests := []struct {
		name   string
		render ops.SensitiveRender
		schema *sdcpb.SchemaElem
		want   string
	}{
		{
			name:   "schema-sensitive, northbound include=false → redact",
			render: ops.NewSensitiveRender(false, nil),
			schema: sensitiveLeaf,
			want:   "***",
		},
		{
			name:   "schema-sensitive, reveal-all → real",
			render: ops.NewSensitiveRender(true, nil),
			schema: sensitiveLeaf,
			want:   "secret",
		},
		{
			name:   "path marker, northbound include=false → redact",
			render: ops.NewSensitiveRender(false, marker),
			schema: plainLeaf,
			want:   "***",
		},
		{
			name:   "plain leaf, no marker → real",
			render: ops.NewSensitiveRender(false, nil),
			schema: plainLeaf,
			want:   "secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			e := mockTreeEntry.NewMockEntry(ctrl)
			e.EXPECT().GetSchema().Return(tt.schema).AnyTimes()
			e.EXPECT().SdcpbPath().Return(path).AnyTimes()

			got := tt.render.TypedValue(e, real)
			if got.GetStringVal() != tt.want {
				t.Errorf("TypedValue() = %q, want %q", got.GetStringVal(), tt.want)
			}
		})
	}
}

func TestSensitiveRender_String(t *testing.T) {
	sensitiveLeaf := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{Field: &sdcpb.LeafSchema{Sensitive: true}},
	}
	ctrl := gomock.NewController(t)
	e := mockTreeEntry.NewMockEntry(ctrl)
	e.EXPECT().GetSchema().Return(sensitiveLeaf).AnyTimes()
	e.EXPECT().SdcpbPath().Return(&sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "pw"}}}).AnyTimes()

	got := ops.NewSensitiveRender(false, nil).String(e, "secret")
	if got != "***" {
		t.Errorf("String() = %q, want %q", got, "***")
	}
}

func TestRenderOptsNorthbound_EmbedsSensitiveRender(t *testing.T) {
	path := &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "pw"}}}
	live := types.NewSensitivePathIndex()
	live.Set("i", []*sdcpb.Path{path})

	opts := ops.RenderOptsNorthbound(false, live)
	ctrl := gomock.NewController(t)
	e := mockTreeEntry.NewMockEntry(ctrl)
	e.EXPECT().GetSchema().Return(&sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{Field: &sdcpb.LeafSchema{Sensitive: false}},
	}).AnyTimes()
	e.EXPECT().SdcpbPath().Return(path).AnyTimes()

	if opts.TypedValue(e, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "x"}}).GetStringVal() != "***" {
		t.Error("RenderOptsNorthbound did not redact path-marker leaf")
	}
}

func TestRenderOptsRevealAll_PassesThrough(t *testing.T) {
	sensitiveLeaf := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{Field: &sdcpb.LeafSchema{Sensitive: true}},
	}
	ctrl := gomock.NewController(t)
	e := mockTreeEntry.NewMockEntry(ctrl)
	e.EXPECT().GetSchema().Return(sensitiveLeaf).AnyTimes()
	e.EXPECT().SdcpbPath().Return(&sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "pw"}}}).AnyTimes()

	opts := ops.RenderOptsRevealAll()
	got := opts.TypedValue(e, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "secret"}})
	if got.GetStringVal() != "secret" {
		t.Errorf("RenderOptsRevealAll TypedValue() = %q, want secret", got.GetStringVal())
	}
}
