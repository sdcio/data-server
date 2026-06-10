package ops_test

import (
	"context"
	"runtime"
	"testing"

	mockTreeEntry "github.com/sdcio/data-server/mocks/mocktreeentry"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	. "github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// fakeTreeContext is a minimal api.TreeContext for GetDeviations tests.
type fakeTreeContext struct{}

func (f *fakeTreeContext) PoolFactory() pool.VirtualPoolFactory         { return nil }
func (f *fakeTreeContext) SchemaClient() schemaClient.SchemaClientBound { return nil }
func (f *fakeTreeContext) DeepCopy() api.TreeContext                    { return f }
func (f *fakeTreeContext) ExplicitDeletes() *api.DeletePathSet          { return nil }
func (f *fakeTreeContext) NonRevertiveInfo() api.NonRevertiveInfos      { return nil }

// leafDeviationEntry builds a LeafVariants with one intent entry (value expectedVal)
// and one running entry (value currentVal) attached to parentEntry.
func leafDeviationEntry(tc api.TreeContext, parentEntry api.Entry, expectedVal, currentVal string) *api.LeafVariants {
	lv := api.NewLeafVariants(tc, parentEntry)

	intentVal := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: expectedVal}}
	lv.Add(api.NewLeafEntry(
		types.NewUpdate(parentEntry, intentVal, 10, "test-intent", 0),
		types.NewUpdateInsertFlags(),
		parentEntry,
	))

	runningVal := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: currentVal}}
	lv.Add(api.NewLeafEntry(
		types.NewUpdate(parentEntry, runningVal, RunningValuesPrio, RunningIntentName, 0),
		types.NewUpdateInsertFlags(),
		parentEntry,
	))

	return lv
}

// collectDeviations runs ops.GetDeviations and returns all emitted DeviationEntries.
func collectDeviations(t *testing.T, entry api.Entry, params *ops.GetDeviationParams) []*types.DeviationEntry {
	t.Helper()
	taskPool := pool.NewSharedTaskPool(context.Background(), runtime.GOMAXPROCS(0))
	ch := make(chan *types.DeviationEntry, 32)
	params.Ch = ch

	err := ops.GetDeviations(context.Background(), entry, params, taskPool)
	close(ch)
	if err != nil {
		t.Fatalf("GetDeviations error: %v", err)
	}

	var result []*types.DeviationEntry
	for de := range ch {
		result = append(result, de)
	}
	return result
}

// setupLeafMock wires a MockEntry to behave as a single leaf node (no children)
// with the given schema and leaf variants.
func setupLeafMock(ctrl *gomock.Controller, schema *sdcpb.SchemaElem, tc api.TreeContext, lv *api.LeafVariants) *mockTreeEntry.MockEntry {
	e := mockTreeEntry.NewMockEntry(ctrl)
	path := &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "test-leaf"}}, IsRootBased: true}
	e.EXPECT().SdcpbPath().Return(path).AnyTimes()
	e.EXPECT().GetSchema().Return(schema).AnyTimes()
	e.EXPECT().GetChilds(types.DescendMethodActiveChilds).Return(api.EntryMap{}).AnyTimes()
	e.EXPECT().GetChildMap().Return(api.NewChildMap()).AnyTimes()
	e.EXPECT().GetLeafVariants().Return(lv).AnyTimes()
	return e
}

// TestGetDeviations_SchemaSensitiveRedacted is the tracer bullet.
// IncludeSensitive=false must replace both expected and current values with "***"
// when the leaf's schema marks it sensitive.
func TestGetDeviations_SchemaSensitiveRedacted(t *testing.T) {
	ctrl := gomock.NewController(t)

	sensitiveSchema := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{Field: &sdcpb.LeafSchema{Sensitive: true}},
	}
	tc := &fakeTreeContext{}

	e := mockTreeEntry.NewMockEntry(ctrl)
	path := &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "secret-leaf"}}, IsRootBased: true}
	e.EXPECT().SdcpbPath().Return(path).AnyTimes()
	e.EXPECT().GetSchema().Return(sensitiveSchema).AnyTimes()
	e.EXPECT().GetChilds(types.DescendMethodActiveChilds).Return(api.EntryMap{}).AnyTimes()
	e.EXPECT().GetChildMap().Return(api.NewChildMap()).AnyTimes()

	lv := leafDeviationEntry(tc, e, "secret-expected", "secret-current")
	e.EXPECT().GetLeafVariants().Return(lv).AnyTimes()

	deviations := collectDeviations(t, e, &ops.GetDeviationParams{RenderOpts: ops.RenderOpts{IncludeSensitive: false}})

	if len(deviations) == 0 {
		t.Fatal("expected at least one deviation, got none")
	}
	for _, de := range deviations {
		if ev := de.ExpectedValue(); ev != nil {
			if got := ev.GetStringVal(); got != "***" {
				t.Errorf("ExpectedValue = %q, want %q", got, "***")
			}
		}
		if cv := de.CurrentValue(); cv != nil {
			if got := cv.GetStringVal(); got != "***" {
				t.Errorf("CurrentValue = %q, want %q", got, "***")
			}
		}
	}
}

// TestGetDeviations_ExposeSensitivePassthrough verifies that IncludeSensitive=true
// leaves schema-sensitive values untouched.
func TestGetDeviations_ExposeSensitivePassthrough(t *testing.T) {
	ctrl := gomock.NewController(t)

	sensitiveSchema := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{Field: &sdcpb.LeafSchema{Sensitive: true}},
	}
	tc := &fakeTreeContext{}

	e := mockTreeEntry.NewMockEntry(ctrl)
	path := &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "secret-leaf"}}, IsRootBased: true}
	e.EXPECT().SdcpbPath().Return(path).AnyTimes()
	e.EXPECT().GetSchema().Return(sensitiveSchema).AnyTimes()
	e.EXPECT().GetChilds(types.DescendMethodActiveChilds).Return(api.EntryMap{}).AnyTimes()
	e.EXPECT().GetChildMap().Return(api.NewChildMap()).AnyTimes()

	lv := leafDeviationEntry(tc, e, "raw-expected", "raw-current")
	e.EXPECT().GetLeafVariants().Return(lv).AnyTimes()

	deviations := collectDeviations(t, e, &ops.GetDeviationParams{RenderOpts: ops.RenderOpts{IncludeSensitive: true}})

	if len(deviations) == 0 {
		t.Fatal("expected at least one deviation, got none")
	}
	de := deviations[0]
	if got := de.ExpectedValue().GetStringVal(); got != "raw-expected" {
		t.Errorf("ExpectedValue = %q, want %q (must not be redacted)", got, "raw-expected")
	}
	if got := de.CurrentValue().GetStringVal(); got != "raw-current" {
		t.Errorf("CurrentValue = %q, want %q (must not be redacted)", got, "raw-current")
	}
}

// TestGetDeviations_IntentMarkerSensitiveRedacted verifies that IncludeSensitive=false
// redacts values when the path appears in GetDeviationParams.SensitivePathSet
// (intent-marker check), even when the schema does not mark it sensitive.
func TestGetDeviations_IntentMarkerSensitiveRedacted(t *testing.T) {
	ctrl := gomock.NewController(t)

	nonSensitiveSchema := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{Field: &sdcpb.LeafSchema{Sensitive: false}},
	}
	leafPath := &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "marker-leaf"}}, IsRootBased: true}

	sps := types.NewSensitivePathIndex()
	sps.Add(leafPath)
	tc := &fakeTreeContext{}

	e := mockTreeEntry.NewMockEntry(ctrl)
	e.EXPECT().SdcpbPath().Return(leafPath).AnyTimes()
	e.EXPECT().GetSchema().Return(nonSensitiveSchema).AnyTimes()
	e.EXPECT().GetChilds(types.DescendMethodActiveChilds).Return(api.EntryMap{}).AnyTimes()
	e.EXPECT().GetChildMap().Return(api.NewChildMap()).AnyTimes()

	lv := leafDeviationEntry(tc, e, "marker-expected", "marker-current")
	e.EXPECT().GetLeafVariants().Return(lv).AnyTimes()

	deviations := collectDeviations(t, e, &ops.GetDeviationParams{RenderOpts: ops.RenderOpts{IncludeSensitive: false, SensitivePathSet: sps}})

	if len(deviations) == 0 {
		t.Fatal("expected at least one deviation, got none")
	}
	for _, de := range deviations {
		if ev := de.ExpectedValue(); ev != nil {
			if got := ev.GetStringVal(); got != "***" {
				t.Errorf("ExpectedValue = %q, want %q (path-marker sensitive)", got, "***")
			}
		}
		if cv := de.CurrentValue(); cv != nil {
			if got := cv.GetStringVal(); got != "***" {
				t.Errorf("CurrentValue = %q, want %q (path-marker sensitive)", got, "***")
			}
		}
	}
}

// TestGetDeviations_NonSensitiveNotRedacted verifies that IncludeSensitive=false
// does NOT redact values for paths that are neither schema-sensitive nor in
// GetDeviationParams.SensitivePathSet.
func TestGetDeviations_NonSensitiveNotRedacted(t *testing.T) {
	ctrl := gomock.NewController(t)

	nonSensitiveSchema := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{Field: &sdcpb.LeafSchema{Sensitive: false}},
	}
	tc := &fakeTreeContext{}

	e := mockTreeEntry.NewMockEntry(ctrl)
	path := &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "plain-leaf"}}, IsRootBased: true}
	e.EXPECT().SdcpbPath().Return(path).AnyTimes()
	e.EXPECT().GetSchema().Return(nonSensitiveSchema).AnyTimes()
	e.EXPECT().GetChilds(types.DescendMethodActiveChilds).Return(api.EntryMap{}).AnyTimes()
	e.EXPECT().GetChildMap().Return(api.NewChildMap()).AnyTimes()

	lv := leafDeviationEntry(tc, e, "plain-expected", "plain-current")
	e.EXPECT().GetLeafVariants().Return(lv).AnyTimes()

	deviations := collectDeviations(t, e, &ops.GetDeviationParams{RenderOpts: ops.RenderOpts{IncludeSensitive: false}})

	if len(deviations) == 0 {
		t.Fatal("expected at least one deviation, got none")
	}
	de := deviations[0]
	if got := de.ExpectedValue().GetStringVal(); got != "plain-expected" {
		t.Errorf("ExpectedValue = %q, want %q (must not be redacted)", got, "plain-expected")
	}
	if got := de.CurrentValue().GetStringVal(); got != "plain-current" {
		t.Errorf("CurrentValue = %q, want %q (must not be redacted)", got, "plain-current")
	}
}
