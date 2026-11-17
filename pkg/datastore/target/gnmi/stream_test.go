package gnmi

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openconfig/gnmi/proto/gnmi"
	schemaClientPkg "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	treeimporter "github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// fakeRunningStore uses real tree creation but intercepts ApplyToRunning.
// The first call blocks until firstUnblock is closed; all subsequent calls
// return immediately so the test can observe the call count.
type fakeRunningStore struct {
	sc *schemaClientPkg.SchemaClientBoundImpl
	vp pool.VirtualPoolFactory

	firstUnblock     chan struct{}
	firstStarted     chan struct{}
	firstStartedOnce sync.Once

	calls atomic.Int32

	mu          sync.Mutex
	deletesSeen [][]*sdcpb.Path
	importSeen  []bool
}

// applied returns a snapshot of the deletes and importer-presence of every completed ApplyToRunning call.
func (f *fakeRunningStore) applied() ([][]*sdcpb.Path, []bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]*sdcpb.Path(nil), f.deletesSeen...), append([]bool(nil), f.importSeen...)
}

func newFakeRunningStore(sc *schemaClientPkg.SchemaClientBoundImpl, vp pool.VirtualPoolFactory) *fakeRunningStore {
	return &fakeRunningStore{
		sc:           sc,
		vp:           vp,
		firstUnblock: make(chan struct{}),
		firstStarted: make(chan struct{}),
	}
}

func (f *fakeRunningStore) ApplyToRunning(ctx context.Context, deletes []*sdcpb.Path, imp treeimporter.ImportConfigAdapter) error {
	f.firstStartedOnce.Do(func() { close(f.firstStarted) })
	select {
	case <-f.firstUnblock:
	case <-ctx.Done():
		return ctx.Err()
	}
	f.mu.Lock()
	f.deletesSeen = append(f.deletesSeen, deletes)
	f.importSeen = append(f.importSeen, imp != nil)
	f.mu.Unlock()
	f.calls.Add(1)
	return nil
}

func (f *fakeRunningStore) NewEmptyTree(ctx context.Context) (*tree.RootEntry, error) {
	tc := tree.NewTreeContext(f.sc, f.vp)
	return tree.NewTreeRoot(ctx, tc)
}

// fakeSyncTarget implements SyncTarget with channels the test controls.
type fakeSyncTarget struct {
	respChan chan *gnmi.SubscribeResponse
	errChan  chan error
}

func (f *fakeSyncTarget) Subscribe(_ context.Context, _ *gnmi.SubscribeRequest, _ string) (chan *gnmi.SubscribeResponse, chan error) {
	return f.respChan, f.errChan
}

// interfaceDescriptionNotif returns a gNMI SubscribeResponse_Update for
// /interface[name=ifname]/description = desc.
func interfaceDescriptionNotif(ifname, desc string) *gnmi.SubscribeResponse {
	return &gnmi.SubscribeResponse{
		Response: &gnmi.SubscribeResponse_Update{
			Update: &gnmi.Notification{
				Update: []*gnmi.Update{
					{
						Path: &gnmi.Path{
							Elem: []*gnmi.PathElem{
								{Name: "interface", Key: map[string]string{"name": ifname}},
								{Name: "description"},
							},
						},
						Val: &gnmi.TypedValue{
							Value: &gnmi.TypedValue_StringVal{StringVal: desc},
						},
					},
				},
			},
		},
	}
}

func syncRespMsg() *gnmi.SubscribeResponse {
	return &gnmi.SubscribeResponse{
		Response: &gnmi.SubscribeResponse_SyncResponse{SyncResponse: true},
	}
}

// TestBuildTreeSyncWithDatastore_PostSyncNotificationsNotDropped is the
// regression test for GitHub issue #440.
//
// Scenario: ApplyToRunning is slow. Notifications that arrive after the
// SyncResponse is processed but while ApplyToRunning is blocked must not be
// dropped — they must be committed by the next ticker.
//
// Observable invariant: ApplyToRunning is called at least twice (once for the
// initial sync commit, and once for the ticker commit of post-sync
// notifications). If notifications were silently dropped, the fresh syncTree
// would be empty and the ticker's syncToRunning would return early without
// calling ApplyToRunning, leaving the count at 1.
func TestBuildTreeSyncWithDatastore_PostSyncNotificationsNotDropped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sc, schemaConf, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatalf("init schema: %v", err)
	}
	scb := schemaClientPkg.NewSchemaClientBound(schemaConf, sc)

	sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	store := newFakeRunningStore(scb, sharedPool)

	respChan := make(chan *gnmi.SubscribeResponse, 20)
	target := &fakeSyncTarget{
		respChan: respChan,
		errChan:  make(chan error, 1),
	}

	ss := NewStreamSync(ctx, target, &config.SyncProtocol{
		Name:  "test",
		Paths: []string{"/"},
		Mode:  "on-change",
	}, store, scb, sharedPool)
	// Zero-buffer channel + short timeout forces a deterministic drop with the
	// buggy implementation: pool workers block immediately (no receiver while
	// the consumer goroutine is in ApplyToRunning) and time out after 20 ms.
	// With the fix the goroutine handoff keeps the main loop draining, so all
	// notifications are delivered before the timeout fires.
	ss.updChanBufSize = 0
	ss.notifSendTimeout = 20 * time.Millisecond

	if err := ss.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := ss.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	}()

	// Initial sync: one notification + SyncResponse.
	respChan <- interfaceDescriptionNotif("ethernet-1/1", "pre-sync")
	respChan <- syncRespMsg()

	// Wait for the first ApplyToRunning call to start (it will block there).
	select {
	case <-store.firstStarted:
	case <-ctx.Done():
		t.Fatal("timed out waiting for first ApplyToRunning to start")
	}

	// While ApplyToRunning is blocked, send post-sync notifications.
	// With the buggy implementation the consumer goroutine is blocked, so
	// pool workers cannot deliver to the zero-buffer updChan and will drop
	// after 20 ms. With the fix the goroutine handoff keeps the consumer
	// running and all notifications reach the fresh syncTree.
	const postSyncCount = 5
	for i := 0; i < postSyncCount; i++ {
		respChan <- interfaceDescriptionNotif("ethernet-1/2", "post-sync")
	}

	// Wait longer than notifSendTimeout so that drops occur with the old code.
	time.Sleep(100 * time.Millisecond)

	// Unblock the first ApplyToRunning.
	close(store.firstUnblock)

	// Wait for the second ApplyToRunning call (ticker commit of post-sync tree).
	// The ticker fires every 5 s; allow up to 3 windows.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if store.calls.Load() >= 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if got := store.calls.Load(); got < 2 {
		t.Errorf("ApplyToRunning called %d time(s); want ≥2 — post-sync notifications appear to have been dropped", got)
	}
}

// TestBuildTreeSyncWithDatastore_NewEmptyTreeFailureExits verifies that
// buildTreeSyncWithDatastore exits cleanly rather than panicking when
// NewEmptyTree fails after the initial sync handoff.
func TestBuildTreeSyncWithDatastore_NewEmptyTreeFailureExits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sc, schemaConf, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatalf("init schema: %v", err)
	}
	scb := schemaClientPkg.NewSchemaClientBound(schemaConf, sc)
	sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

	store := &failAfterFirstNewEmptyTree{
		inner: newFakeRunningStore(scb, sharedPool),
	}

	respChan := make(chan *gnmi.SubscribeResponse, 10)
	target := &fakeSyncTarget{
		respChan: respChan,
		errChan:  make(chan error, 1),
	}

	ss := NewStreamSync(ctx, target, &config.SyncProtocol{
		Name:  "test",
		Paths: []string{"/"},
		Mode:  "on-change",
	}, store, scb, sharedPool)
	ss.updChanBufSize = 0
	ss.notifSendTimeout = 20 * time.Millisecond

	if err := ss.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := ss.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	}()

	// Close the inner firstUnblock immediately so the first ApplyToRunning
	// does not stall; we just want NewEmptyTree to fail on the second call.
	close(store.inner.firstUnblock)

	respChan <- interfaceDescriptionNotif("ethernet-1/1", "init")
	respChan <- syncRespMsg()

	// The goroutine should exit within a short timeout; the test passes if
	// there is no panic and the context is not leaked.
	time.Sleep(2 * time.Second)
}

// failAfterFirstNewEmptyTree wraps fakeRunningStore but returns an error on
// the second NewEmptyTree call (simulating the post-handoff allocation failure).
type failAfterFirstNewEmptyTree struct {
	inner *fakeRunningStore
	calls atomic.Int32
}

func (f *failAfterFirstNewEmptyTree) ApplyToRunning(ctx context.Context, d []*sdcpb.Path, i treeimporter.ImportConfigAdapter) error {
	return f.inner.ApplyToRunning(ctx, d, i)
}

func (f *failAfterFirstNewEmptyTree) NewEmptyTree(ctx context.Context) (*tree.RootEntry, error) {
	if f.calls.Add(1) > 1 {
		return nil, context.DeadlineExceeded
	}
	return f.inner.NewEmptyTree(ctx)
}

// newStreamSyncForTest wires a StreamSync to a fake target and running store
// and returns the pieces a test needs to drive it. firstUnblock is already
// closed so ApplyToRunning never stalls.
func newStreamSyncForTest(t *testing.T, ctx context.Context, paths []string) (*StreamSync, *fakeSyncTarget, *fakeRunningStore) {
	t.Helper()

	sc, schemaConf, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatalf("init schema: %v", err)
	}
	scb := schemaClientPkg.NewSchemaClientBound(schemaConf, sc)
	sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

	store := newFakeRunningStore(scb, sharedPool)
	close(store.firstUnblock)

	target := &fakeSyncTarget{
		respChan: make(chan *gnmi.SubscribeResponse, 20),
		errChan:  make(chan error, 1),
	}

	ss := NewStreamSync(ctx, target, &config.SyncProtocol{
		Name:  "test",
		Paths: paths,
		Mode:  "on-change",
	}, store, scb, sharedPool)

	if err := ss.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := ss.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})
	return ss, target, store
}

func waitForApplyCalls(t *testing.T, store *fakeRunningStore, want int32) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if store.calls.Load() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("ApplyToRunning called %d time(s); want >= %d", store.calls.Load(), want)
}

// TestGnmiSubscribe_StreamErrorKeepsSyncAlive verifies that an error reported
// by the target's subscription (which retries on its own) does not end the
// sync: notifications delivered after the error are still committed.
func TestGnmiSubscribe_StreamErrorKeepsSyncAlive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, target, store := newStreamSyncForTest(t, ctx, []string{"/interface"})

	target.errChan <- errors.New("rpc error: code = Unavailable desc = connection closed")
	target.respChan <- interfaceDescriptionNotif("ethernet-1/1", "after-restart")
	target.respChan <- syncRespMsg()

	waitForApplyCalls(t, store, 1)
}

// TestStreamSync_ResubscribeReplacesSyncedPaths verifies that the initial sync
// following a stream error replaces the configured sync paths in running
// (so config lost on the device is not retained), while the very first sync
// only merges.
func TestStreamSync_ResubscribeReplacesSyncedPaths(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, target, store := newStreamSyncForTest(t, ctx, []string{"/interface"})

	// first sync: merge only
	target.respChan <- interfaceDescriptionNotif("ethernet-1/1", "initial")
	target.respChan <- syncRespMsg()
	waitForApplyCalls(t, store, 1)

	// device restarts and comes back with empty config
	target.errChan <- errors.New("rpc error: code = Unavailable")
	target.respChan <- syncRespMsg()
	waitForApplyCalls(t, store, 2)

	// device restarts again and comes back with config
	target.errChan <- errors.New("rpc error: code = Unavailable")
	target.respChan <- interfaceDescriptionNotif("ethernet-1/1", "again")
	target.respChan <- syncRespMsg()
	waitForApplyCalls(t, store, 3)

	deletes, imported := store.applied()
	if len(deletes) != 3 {
		t.Fatalf("got %d ApplyToRunning calls, want 3", len(deletes))
	}

	if len(deletes[0]) != 0 || !imported[0] {
		t.Errorf("first sync: deletes=%v import=%v, want no deletes and an import", deletes[0], imported[0])
	}
	for i := 1; i <= 2; i++ {
		if len(deletes[i]) != 1 || deletes[i][0].ToXPath(false) != "/interface" {
			t.Errorf("resync %d: deletes=%v, want [/interface]", i, deletes[i])
		}
	}
	if imported[1] {
		t.Errorf("resync with empty device config must not import anything")
	}
	if !imported[2] {
		t.Errorf("resync with device config must import it")
	}
}
