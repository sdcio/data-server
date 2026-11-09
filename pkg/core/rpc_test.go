package core

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/datastore"
	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/encoding/prototext"
)

const ifPath = "/interface[name=ethernet-1/1]"

func ifIntent(name, descr string) api.Intent {
	return api.Intent{Name: name, Spec: api.IntentSpec{Update: &api.Update{
		Priority: 10,
		Entries:  []api.UpdateEntry{{Path: ifPath, Content: []byte(`{"description":"` + descr + `"}`)}},
	}}}
}

func TestTransactionLifecycleWithoutPeer(t *testing.T) {
	c := startCore(t)
	ctx := context.Background()
	ref := api.TxRef{Datastore: "ds1", ID: "tx1"}

	res, err := c.TransactionSet(ctx, ref, api.SetOptions{Origin: "test"}, []api.Intent{ifIntent("i1", "one")})
	if err != nil {
		t.Fatal(err)
	}
	for name, iv := range res.Intents {
		if len(iv.Errors) > 0 {
			t.Fatalf("intent %s: %v", name, iv.Errors)
		}
	}
	if len(res.Updates) == 0 {
		t.Fatalf("expected updates, got %+v", res)
	}
	if err := c.TransactionConfirm(ctx, ref, "test"); err != nil {
		t.Fatal(err)
	}

	got, err := c.GetIntent(ctx, api.IntentRequest{Datastore: "ds1", Intent: "i1", Format: api.FormatJSON})
	if err != nil {
		t.Fatal(err)
	}
	if got.Priority != 10 || !strings.Contains(string(got.Content), "one") {
		t.Fatalf("unexpected intent: %+v %s", got, got.Content)
	}

	ref2 := api.TxRef{Datastore: "ds1", ID: "tx2"}
	if _, err := c.TransactionSet(ctx, ref2, api.SetOptions{}, []api.Intent{ifIntent("i1", "two")}); err != nil {
		t.Fatal(err)
	}
	if err := c.TransactionCancel(ctx, ref2, ""); err != nil {
		t.Fatal(err)
	}
	got, err = c.GetIntent(ctx, api.IntentRequest{Datastore: "ds1", Intent: "i1", Format: api.FormatJSON})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got.Content), "one") {
		t.Fatalf("cancel did not roll back: %s", got.Content)
	}

	blame, err := c.BlameConfig(ctx, "ds1", false)
	if err != nil {
		t.Fatal(err)
	}
	if blame == nil || blame.Name != "ds1" {
		t.Fatalf("blame = %+v", blame)
	}
}

func TestTransactionDryRunAndDelete(t *testing.T) {
	c := startCore(t)
	ctx := context.Background()

	if _, err := c.TransactionSet(ctx, api.TxRef{Datastore: "ds1", ID: "dry"}, api.SetOptions{DryRun: true}, []api.Intent{ifIntent("d1", "x")}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetIntent(ctx, api.IntentRequest{Datastore: "ds1", Intent: "d1"}); err == nil {
		t.Fatal("dry run must not store the intent")
	}

	if _, err := c.TransactionSet(ctx, api.TxRef{Datastore: "ds1", ID: "t1"}, api.SetOptions{}, []api.Intent{ifIntent("d1", "x")}); err != nil {
		t.Fatal(err)
	}
	if err := c.TransactionConfirm(ctx, api.TxRef{Datastore: "ds1", ID: "t1"}, ""); err != nil {
		t.Fatal(err)
	}
	del := api.Intent{Name: "d1", Spec: api.IntentSpec{Delete: &api.Delete{IgnoreNoExist: true}}}
	if _, err := c.TransactionSet(ctx, api.TxRef{Datastore: "ds1", ID: "t2"}, api.SetOptions{}, []api.Intent{del}); err != nil {
		t.Fatal(err)
	}
	if err := c.TransactionConfirm(ctx, api.TxRef{Datastore: "ds1", ID: "t2"}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetIntent(ctx, api.IntentRequest{Datastore: "ds1", Intent: "d1"}); err == nil {
		t.Fatal("intent should be deleted")
	}
}

func TestTransactionArgumentErrors(t *testing.T) {
	c := startCore(t)
	ctx := context.Background()

	if _, err := c.TransactionSet(ctx, api.TxRef{ID: "x"}, api.SetOptions{}, nil); !errors.Is(err, api.ErrInvalidArgument) {
		t.Fatalf("empty datastore: %v", err)
	}
	if _, err := c.TransactionSet(ctx, api.TxRef{Datastore: "nope"}, api.SetOptions{}, nil); !errors.Is(err, api.ErrUnknownDatastore) {
		t.Fatalf("unknown datastore: %v", err)
	}
	if err := c.TransactionConfirm(ctx, api.TxRef{Datastore: "nope"}, ""); !errors.Is(err, api.ErrUnknownDatastore) {
		t.Fatalf("confirm unknown: %v", err)
	}
	if err := c.TransactionCancel(ctx, api.TxRef{}, ""); !errors.Is(err, api.ErrInvalidArgument) {
		t.Fatalf("cancel empty: %v", err)
	}
	in := ifIntent("p", "x")
	in.Spec.Update.Priority = 1<<31 - 1
	if _, err := c.TransactionSet(ctx, api.TxRef{Datastore: "ds1", ID: "p"}, api.SetOptions{}, []api.Intent{in}); !errors.Is(err, api.ErrInvalidArgument) {
		t.Fatalf("priority: %v", err)
	}
	if _, err := c.GetIntent(ctx, api.IntentRequest{Datastore: "ds1"}); !errors.Is(err, api.ErrInvalidArgument) {
		t.Fatalf("missing intent name: %v", err)
	}
}

// The datastore and target packages report the api sentinels, so callers can
// match them with errors.Is and api.IsRetryable.
func TestSentinelsAreShared(t *testing.T) {
	if !errors.Is(datastore.ErrDatastoreLocked, api.ErrDatastoreLocked) || !api.IsRetryable(datastore.ErrDatastoreLocked) {
		t.Fatal("datastore.ErrDatastoreLocked is not api.ErrDatastoreLocked")
	}
	if !errors.Is(targettypes.ErrNotConnected, api.ErrNotConnected) || !api.IsRetryable(targettypes.ErrNotConnected) {
		t.Fatal("targettypes.ErrNotConnected is not api.ErrNotConnected")
	}
}

func TestTransactionSetNotConnected(t *testing.T) {
	c := startCore(t)
	ctx := context.Background()
	spec := api.DatastoreSpec{
		Name:   "offline",
		Target: api.Target{Type: api.TargetTypeGNMI, Address: "127.0.0.1", Port: 1, ProtocolOptions: api.ProtocolOptions{GNMI: &api.GNMIOptions{Encoding: "JSON_IETF"}}},
		Schema: api.SchemaRef{Vendor: "v", Version: "1"},
	}
	if err := c.CreateDataStore(ctx, spec); err != nil {
		t.Fatal(err)
	}
	_, err := c.TransactionSet(ctx, api.TxRef{Datastore: "offline", ID: "n1"}, api.SetOptions{}, []api.Intent{ifIntent("i", "x")})
	if !errors.Is(err, api.ErrNotConnected) || !api.IsRetryable(err) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateDeleteDataStore(t *testing.T) {
	c := startCore(t)
	ctx := context.Background()
	spec := api.DatastoreSpec{
		Name:   "ds2",
		Target: api.Target{Type: "noop"},
		Schema: api.SchemaRef{Vendor: "v", Version: "1"},
	}
	if err := c.CreateDataStore(ctx, spec); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetDataStore(ctx, "ds2"); err != nil {
		t.Fatal(err)
	}
	if err := c.CreateDataStore(ctx, spec); !errors.Is(err, api.ErrInvalidArgument) {
		t.Fatalf("duplicate: %v", err)
	}
	bad := spec
	bad.Name = "bad"
	bad.Target.Type = "carrier-pigeon"
	if err := c.CreateDataStore(ctx, bad); !errors.Is(err, api.ErrInvalidArgument) {
		t.Fatalf("bad target: %v", err)
	}
	if err := c.CreateDataStore(ctx, api.DatastoreSpec{}); !errors.Is(err, api.ErrInvalidArgument) {
		t.Fatalf("empty name: %v", err)
	}

	if err := c.DeleteDataStore(ctx, "ds2"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetDataStore(ctx, "ds2"); !errors.Is(err, api.ErrUnknownDatastore) {
		t.Fatalf("after delete: %v", err)
	}
	if err := c.DeleteDataStore(ctx, "ds2"); !errors.Is(err, api.ErrUnknownDatastore) {
		t.Fatalf("second delete: %v", err)
	}
	if err := c.DeleteDataStore(ctx, ""); !errors.Is(err, api.ErrInvalidArgument) {
		t.Fatalf("empty delete: %v", err)
	}
}

func TestSchemaDetails(t *testing.T) {
	c := startCore(t)
	info, err := c.GetSchemaDetails(context.Background(), api.SchemaRef{Vendor: "v", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Schema.Vendor != "v" || info.State != api.SchemaReady || len(info.Files) == 0 {
		t.Fatalf("unexpected: %+v", info)
	}
}

func TestSchemaCreateDelete(t *testing.T) {
	c := startCore(t)
	ctx := context.Background()
	ref := api.SchemaRef{Vendor: "v2", Version: "2"}
	err := c.CreateSchema(ctx, api.CreateSchemaSpec{Schema: ref, Files: []string{"../../tests/schema/sdcio_model_if.yang"}, Directories: []string{"../../tests/schema"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteSchema(ctx, api.DeleteSchemaSpec{Schema: ref}); err != nil {
		t.Fatal(err)
	}
}

func recvEvent(t *testing.T, ch <-chan api.DeviationEvent) (api.DeviationEvent, bool) {
	t.Helper()
	select {
	case ev, ok := <-ch:
		return ev, ok
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event")
		return api.DeviationEvent{}, false
	}
}

func TestWatchDeviationsClosesOnDatastoreDelete(t *testing.T) {
	c := startCore(t)
	ctx := context.Background()
	if err := c.CreateDataStore(ctx, api.DatastoreSpec{Name: "w", Target: api.Target{Type: "noop"}, Schema: api.SchemaRef{Vendor: "v", Version: "1"}}); err != nil {
		t.Fatal(err)
	}
	ch, err := c.WatchDeviations(ctx, "w")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteDataStore(ctx, "w"); err != nil {
		t.Fatal(err)
	}
	if _, ok := recvEvent(t, ch); ok {
		t.Fatal("unexpected event")
	}
}

func TestWatchDeviationsClosesOnCoreStop(t *testing.T) {
	c := startCore(t)
	ch, err := c.WatchDeviations(context.Background(), "ds1")
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Stop(context.Background())
	if _, ok := recvEvent(t, ch); ok {
		t.Fatal("unexpected event")
	}
}

func newTestWatcher(ctx context.Context, warnAfter time.Duration, logs *syncBuf) *deviationWatcher {
	log := funcr.New(func(prefix, args string) { logs.add(prefix + " " + args) }, funcr.Options{Verbosity: 2})
	return &deviationWatcher{
		datastore: "ds1",
		ctx:       ctx,
		coreCtx:   context.Background(),
		ch:        make(chan api.DeviationEvent, deviationBuffer),
		warnAfter: warnAfter,
		log:       log,
	}
}

type syncBuf struct {
	mu    sync.Mutex
	lines []string
}

func (b *syncBuf) add(s string) { b.mu.Lock(); b.lines = append(b.lines, s); b.mu.Unlock() }
func (b *syncBuf) text() string { b.mu.Lock(); defer b.mu.Unlock(); return strings.Join(b.lines, "\n") }

func sdcpbEvent(e sdcpb.DeviationEvent) *sdcpb.WatchDeviationResponse {
	return &sdcpb.WatchDeviationResponse{Name: "ds1", Event: e}
}

func TestDeviationWatcherIgnoresDuplicateStart(t *testing.T) {
	logs := &syncBuf{}
	w := newTestWatcher(context.Background(), time.Hour, logs)
	for _, e := range []sdcpb.DeviationEvent{sdcpb.DeviationEvent_START, sdcpb.DeviationEvent_START, sdcpb.DeviationEvent_END, sdcpb.DeviationEvent_START} {
		if err := w.Send(sdcpbEvent(e)); err != nil {
			t.Fatal(err)
		}
	}
	var kinds []api.DeviationKind
	for len(w.ch) > 0 {
		kinds = append(kinds, (<-w.ch).Kind)
	}
	want := []api.DeviationKind{api.DeviationStart, api.DeviationEnd, api.DeviationStart}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", kinds, want)
		}
	}
	if !strings.Contains(logs.text(), "duplicate deviation START ignored") {
		t.Fatalf("duplicate START not logged: %s", logs.text())
	}
}

func TestDeviationWatcherBlocksWhenFullAndWarns(t *testing.T) {
	logs := &syncBuf{}
	w := newTestWatcher(context.Background(), 50*time.Millisecond, logs)
	for i := 0; i < deviationBuffer; i++ {
		if err := w.Send(sdcpbEvent(sdcpb.DeviationEvent_UPDATE)); err != nil {
			t.Fatal(err)
		}
	}

	sent := make(chan error, 1)
	go func() { sent <- w.Send(sdcpbEvent(sdcpb.DeviationEvent_UPDATE)) }()

	select {
	case err := <-sent:
		t.Fatalf("send must block on a full channel, returned %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if !strings.Contains(logs.text(), "has not consumed an event") {
		t.Fatalf("stall warning not logged: %s", logs.text())
	}

	<-w.ch
	select {
	case err := <-sent:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("send did not resume after the watcher read")
	}
	if len(w.ch) != deviationBuffer {
		t.Fatalf("nothing may be dropped, buffered = %d", len(w.ch))
	}
}

func TestDeviationWatcherSendUnblocksOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := newTestWatcher(ctx, time.Hour, &syncBuf{})
	for i := 0; i < deviationBuffer; i++ {
		_ = w.Send(sdcpbEvent(sdcpb.DeviationEvent_UPDATE))
	}
	sent := make(chan error, 1)
	go func() { sent <- w.Send(sdcpbEvent(sdcpb.DeviationEvent_UPDATE)) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-sent:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("send stayed blocked after cancel")
	}
	w.close()
	w.close()
	if err := w.Send(sdcpbEvent(sdcpb.DeviationEvent_UPDATE)); err == nil {
		t.Fatal("send after close must fail")
	}
}

func TestToDeviationEventEntryText(t *testing.T) {
	rsp := &sdcpb.WatchDeviationResponse{
		Event:         sdcpb.DeviationEvent_UPDATE,
		ExpectedValue: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "a"}},
	}
	e := toDeviationEvent(rsp).Entries[0]
	var tv sdcpb.TypedValue
	if err := prototext.Unmarshal([]byte(e.ExpectedText), &tv); err != nil {
		t.Fatalf("ExpectedText is not prototext: %v", err)
	}
	if tv.GetStringVal() != "a" {
		t.Fatalf("round trip mismatch: %v", &tv)
	}
	if e.CurrentText != "" {
		t.Fatalf("absent value must be empty, got %q", e.CurrentText)
	}
}
