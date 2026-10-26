package ops_test

import (
	"testing"

	mockTreeEntry "github.com/sdcio/data-server/mocks/mocktreeentry"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/sensitive"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

func TestRenderOptsNorthbound_RedactsPathMarkedLeaf(t *testing.T) {
	path := &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "pw"}}}
	live := sensitive.NewPathIndex()
	live.Set("i", []*sdcpb.Path{path})

	opts := ops.RenderOptsNorthbound(false, live)
	ctrl := gomock.NewController(t)
	e := mockTreeEntry.NewMockEntry(ctrl)
	e.EXPECT().GetSchema().Return(&sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{Field: &sdcpb.LeafSchema{Sensitive: false}},
	}).AnyTimes()
	e.EXPECT().SdcpbPath().Return(path).AnyTimes()

	got := opts.TypedValue(e, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "x"}})
	if got.GetStringVal() != sensitive.RedactedStringValue {
		t.Error("RenderOptsNorthbound did not redact path-marker leaf")
	}
}

func TestRenderOptsRevealAll_PassesThrough(t *testing.T) {
	ctrl := gomock.NewController(t)
	e := mockTreeEntry.NewMockEntry(ctrl)
	e.EXPECT().GetSchema().Return(&sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{Field: &sdcpb.LeafSchema{Sensitive: true}},
	}).AnyTimes()
	e.EXPECT().SdcpbPath().Return(&sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "pw"}}}).AnyTimes()

	got := ops.RenderOptsRevealAll().TypedValue(e, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "secret"}})
	if got.GetStringVal() != "secret" {
		t.Errorf("RenderOptsRevealAll TypedValue() = %q, want secret", got.GetStringVal())
	}
}
