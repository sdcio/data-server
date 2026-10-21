package sensitive_test

import (
	"testing"

	"github.com/sdcio/data-server/pkg/tree/sensitive"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type fakeLeaf struct {
	schema *sdcpb.SchemaElem
	path   *sdcpb.Path
}

func (f fakeLeaf) GetSchema() *sdcpb.SchemaElem { return f.schema }
func (f fakeLeaf) SdcpbPath() *sdcpb.Path       { return f.path }

func schemaWith(sensitiveFlag bool) *sdcpb.SchemaElem {
	return &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{Field: &sdcpb.LeafSchema{Sensitive: sensitiveFlag}},
	}
}

func TestRender_TypedValue(t *testing.T) {
	real := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "secret"}}
	path := &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "secret"}}}
	marker := sensitive.NewPathIndex()
	marker.Set("intent", []*sdcpb.Path{path})

	tests := []struct {
		name   string
		render sensitive.Render
		schema *sdcpb.SchemaElem
		want   string
	}{
		{"schema-sensitive, include=false → redact", sensitive.NewRender(false, nil), schemaWith(true), "***"},
		{"schema-sensitive, include=true → real", sensitive.NewRender(true, nil), schemaWith(true), "secret"},
		{"path marker, include=false → redact", sensitive.NewRender(false, marker), schemaWith(false), "***"},
		{"path marker, include=true → real", sensitive.NewRender(true, marker), schemaWith(false), "secret"},
		{"plain leaf, no marker → real", sensitive.NewRender(false, nil), schemaWith(false), "secret"},
		{"nil schema, no marker → real", sensitive.NewRender(false, nil), nil, "secret"},
		{"RevealAll never redacts", sensitive.RevealAll(), schemaWith(true), "secret"},
		{"zero value redacts schema-sensitive", sensitive.Render{}, schemaWith(true), "***"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.render.TypedValue(fakeLeaf{schema: tt.schema, path: path}, real)
			if got.GetStringVal() != tt.want {
				t.Errorf("TypedValue() = %q, want %q", got.GetStringVal(), tt.want)
			}
		})
	}
}

func TestRender_String(t *testing.T) {
	leaf := fakeLeaf{schema: schemaWith(true), path: &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "pw"}}}}

	if got := sensitive.NewRender(false, nil).String(leaf, "secret"); got != sensitive.RedactedStringValue {
		t.Errorf("String() = %q, want %q", got, sensitive.RedactedStringValue)
	}
	if got := sensitive.RevealAll().String(leaf, "secret"); got != "secret" {
		t.Errorf("RevealAll String() = %q, want secret", got)
	}
}

// alwaysContains is a PathChecker that treats every path as marked.
type alwaysContains struct{}

func (alwaysContains) Contains(*sdcpb.Path) bool { return true }

func TestRender_ShouldRedact(t *testing.T) {
	somePath := &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "x"}}}

	sensitiveLeaf := schemaWith(true)
	plainLeaf := schemaWith(false)

	tests := []struct {
		name             string
		schema           *sdcpb.SchemaElem
		includeSensitive bool
		paths            sensitive.PathChecker
		want             bool
	}{
		{name: "schema-sensitive leaf, include=false → redact", schema: sensitiveLeaf, want: true},
		{name: "schema-sensitive leaf, include=true → reveal", schema: sensitiveLeaf, includeSensitive: true},
		{name: "non-sensitive leaf → pass through", schema: plainLeaf},
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
		},
		{name: "nil schema → pass through"},
		{name: "path marker → redact", schema: plainLeaf, paths: alwaysContains{}, want: true},
		{name: "path marker, include=true → reveal", schema: plainLeaf, paths: alwaysContains{}, includeSensitive: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sensitive.NewRender(tt.includeSensitive, tt.paths).ShouldRedact(fakeLeaf{schema: tt.schema, path: somePath})
			if got != tt.want {
				t.Errorf("ShouldRedact() = %v, want %v", got, tt.want)
			}
		})
	}
}
