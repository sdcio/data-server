package datastore

import (
	"context"
	"runtime"
	"sync"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/mocks/mockcacheclient"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/pool"
	schemaClientPkg "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	tree_persist "github.com/sdcio/sdc-protos/tree_persist"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/metadata"
)

// fakeDeviationStream is a minimal in-memory DataServer_WatchDeviationsServer.
// Only Send and Context are called by SendDeviations; everything else is a no-op.
type fakeDeviationStream struct {
	ctx      context.Context
	mu       sync.Mutex
	received []*sdcpb.WatchDeviationResponse
}

func newFakeDeviationStream(ctx context.Context) *fakeDeviationStream {
	return &fakeDeviationStream{ctx: ctx}
}

func (f *fakeDeviationStream) Send(r *sdcpb.WatchDeviationResponse) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.received = append(f.received, r)
	return nil
}

func (f *fakeDeviationStream) updateResponses() []*sdcpb.WatchDeviationResponse {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*sdcpb.WatchDeviationResponse
	for _, r := range f.received {
		if r.GetEvent() == sdcpb.DeviationEvent_UPDATE {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeDeviationStream) Context() context.Context     { return f.ctx }
func (f *fakeDeviationStream) SetHeader(metadata.MD) error  { return nil }
func (f *fakeDeviationStream) SendHeader(metadata.MD) error { return nil }
func (f *fakeDeviationStream) SetTrailer(metadata.MD)       {}
func (f *fakeDeviationStream) SendMsg(any) error            { return nil }
func (f *fakeDeviationStream) RecvMsg(any) error            { return nil }

var _ sdcpb.DataServer_WatchDeviationsServer = (*fakeDeviationStream)(nil)

// deviationEntry builds a DeviationEntry with string-valued ExpectedValue and CurrentValue.
func deviationEntry(path *sdcpb.Path, expected, current string) *treetypes.DeviationEntry {
	return treetypes.NewDeviationEntry("test-intent", treetypes.DeviationReasonNotApplied, path).
		SetExpectedValue(&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: expected}}).
		SetCurrentValue(&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: current}})
}

// simplePath builds a root-based sdcpb.Path with a single element.
func simplePath(name string) *sdcpb.Path {
	return &sdcpb.Path{
		IsRootBased: true,
		Elem:        []*sdcpb.PathElem{{Name: name}},
	}
}

// TestSendDeviations_PassthroughUnredacted verifies that SendDeviations forwards
// deviation entries to clients without modifying values. Sensitivity redaction is
// now the responsibility of the ops layer (GetDeviations), not the datastore layer.
func TestSendDeviations_PassthroughUnredacted(t *testing.T) {
	ctx := context.Background()

	ds := &Datastore{
		config: &config.DatastoreConfig{Name: "test-ds", Validation: config.NewValidationConfig()},
	}

	path := simplePath("plain-leaf")
	ch := make(chan *treetypes.DeviationEntry, 1)
	ch <- deviationEntry(path, "expected-value", "current-value")
	close(ch)

	stream := newFakeDeviationStream(ctx)
	ds.SendDeviations(ctx, ch, map[string]sdcpb.DataServer_WatchDeviationsServer{"fake": stream})

	updates := stream.updateResponses()
	if len(updates) != 1 {
		t.Fatalf("expected 1 UPDATE response, got %d", len(updates))
	}
	if got := updates[0].GetExpectedValue().GetStringVal(); got != "expected-value" {
		t.Errorf("ExpectedValue = %q, want %q", got, "expected-value")
	}
	if got := updates[0].GetCurrentValue().GetStringVal(); got != "current-value" {
		t.Errorf("CurrentValue = %q, want %q", got, "current-value")
	}
}

// TestWatchDeviations_SensitivePathMasking is the integration acceptance test
// for Issue 05.
//
// Scenario: one intent sets "patterntest" to "running-secret"; a second (marker)
// intent marks /patterntest in its sensitive_paths. The sync tree has no value
// for patterntest, so the deviation engine sees a mismatch. Both ExpectedValue
// and CurrentValue in the emitted deviation must be "***".
func TestWatchDeviations_SensitivePathMasking(t *testing.T) {
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	// Plain schema client — patterntest is NOT schema-sensitive.
	scb := schemaClientPkg.NewSchemaClientBound(schema, sc)

	runningDevice := &sdcio_schema.Device{Patterntest: ygot.String("running-secret")}
	dataIntent := buildTestIntent(t, ctx, scb, runningDevice, "data-intent", 10, nil)
	markerIntent := buildTestIntent(t, ctx, scb, nil, "marker-intent", 5, []*sdcpb.Path{{Elem: []*sdcpb.PathElem{{Name: "patterntest"}}, IsRootBased: true}})

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
	ccb.EXPECT().
		IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
			intentChan <- dataIntent
			intentChan <- markerIntent
			close(intentChan)
			close(errChan)
		})

	// Pre-populate index with marker-intent's sensitive paths.
	idx := treetypes.NewSensitivePathIndex()
	idx.Set("marker-intent", markerIntent.GetSensitivePaths())

	// Empty sync tree → running device has no patterntest → triggers deviation.
	syncRoot := buildEmptySyncTree(t, ctx, scb)

	ds := &Datastore{
		config:             &config.DatastoreConfig{Name: "test-ds", Validation: config.NewValidationConfig()},
		syncTree:           syncRoot,
		syncTreeMutex:      &sync.RWMutex{},
		taskPool:           pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)),
		cacheClient:        ccb,
		schemaClient:       scb,
		sensitivePathIndex: idx,
	}

	deviationChan, calcErr := ds.calculateDeviations(ctx)
	if calcErr != nil {
		t.Fatalf("calculateDeviations() error: %v", calcErr)
	}

	stream := newFakeDeviationStream(ctx)
	ds.SendDeviations(ctx, deviationChan, map[string]sdcpb.DataServer_WatchDeviationsServer{"fake": stream})

	found := false
	for _, r := range stream.updateResponses() {
		elems := r.GetPath().GetElem()
		if len(elems) > 0 && elems[len(elems)-1].GetName() == "patterntest" {
			found = true
			if got := r.GetExpectedValue().GetStringVal(); got != "***" {
				t.Errorf("patterntest ExpectedValue = %q, want %q", got, "***")
			}
			if got := r.GetCurrentValue().GetStringVal(); got != "***" {
				t.Errorf("patterntest CurrentValue = %q, want %q", got, "***")
			}
		}
	}
	if !found {
		t.Error("no deviation found for patterntest — check that the deviation engine emits it")
	}
}
