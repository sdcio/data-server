package gnmi

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/openconfig/gnmi/proto/gnmi"
	schemaClientPkg "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
)

type countingFakeSyncTarget struct {
	fakeSyncTarget
	subscribes chan struct{}
}

func newCountingFakeSyncTarget(respChan chan *gnmi.SubscribeResponse, errChan chan error) *countingFakeSyncTarget {
	return &countingFakeSyncTarget{
		fakeSyncTarget: fakeSyncTarget{respChan: respChan, errChan: errChan},
		subscribes:     make(chan struct{}, 10),
	}
}

func (f *countingFakeSyncTarget) Subscribe(ctx context.Context, req *gnmi.SubscribeRequest, name string) (chan *gnmi.SubscribeResponse, chan error) {
	select {
	case f.subscribes <- struct{}{}:
	default:
	}
	return f.fakeSyncTarget.Subscribe(ctx, req, name)
}

func TestOnceSync_MarksSyncedAfterSuccessfulCycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sc, schemaConf, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatalf("init schema: %v", err)
	}
	scb := schemaClientPkg.NewSchemaClientBound(schemaConf, sc)
	sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

	const syncName = "once-sync"
	c := &config.SyncProtocol{
		Name:     syncName,
		Mode:     "once",
		Paths:    []string{"/interface"},
		Interval: time.Hour,
	}

	t.Run("successful ONCE with data marks Synced", func(t *testing.T) {
		store := newFakeRunningStore(scb, sharedPool)
		close(store.firstUnblock)

		respChan := make(chan *gnmi.SubscribeResponse, 10)
		target := &fakeSyncTarget{
			respChan: respChan,
			errChan:  make(chan error, 1),
		}

		os, err := NewOnceSync(ctx, target, c, store, scb, sharedPool)
		if err != nil {
			t.Fatalf("NewOnceSync: %v", err)
		}
		defer func() { _ = os.Stop() }()

		if err := os.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}

		respChan <- interfaceDescriptionNotif("ethernet-1/1", "once description")
		respChan <- syncRespMsg()

		waitUntilSynced(t, store, syncName)
	})

	t.Run("empty SyncResponse applies scoped refresh and marks Synced", func(t *testing.T) {
		store := newRecordingApplyRunningStore(scb, sharedPool)
		close(store.firstUnblock)

		respChan := make(chan *gnmi.SubscribeResponse, 10)
		target := &fakeSyncTarget{
			respChan: respChan,
			errChan:  make(chan error, 1),
		}

		os, err := NewOnceSync(ctx, target, c, store, scb, sharedPool)
		if err != nil {
			t.Fatalf("NewOnceSync: %v", err)
		}
		defer func() { _ = os.Stop() }()

		if err := os.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}

		respChan <- syncRespMsg()

		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			called, _, _ := store.applySnapshot()
			if called && store.isSynced(syncName) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		called, importerNil, pathCount := store.applySnapshot()
		if !called {
			t.Fatal("ApplyToRunning was not called for empty ONCE cycle")
		}
		if !importerNil {
			t.Error("ApplyToRunning importer: want nil for empty export")
		}
		if pathCount != len(c.Paths) {
			t.Errorf("ApplyToRunning paths: got %d, want %d (configured sync paths)", pathCount, len(c.Paths))
		}
		if !store.isSynced(syncName) {
			t.Errorf("runningStore.MarkSynced(%q) was not called after empty ONCE cycle", syncName)
		}
	})

	t.Run("subscribe error does not mark Synced", func(t *testing.T) {
		store := newFakeRunningStore(scb, sharedPool)
		close(store.firstUnblock)

		respChan := make(chan *gnmi.SubscribeResponse)
		errChan := make(chan error, 1)
		target := &fakeSyncTarget{respChan: respChan, errChan: errChan}

		os, err := NewOnceSync(ctx, target, c, store, scb, sharedPool)
		if err != nil {
			t.Fatalf("NewOnceSync: %v", err)
		}
		defer func() { _ = os.Stop() }()

		if err := os.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}

		errChan <- errors.New("subscribe failed")
		time.Sleep(300 * time.Millisecond)

		if store.isSynced(syncName) {
			t.Error("runningStore reports Synced after subscribe failure")
		}
	})

	t.Run("stream ends without SyncResponse does not mark Synced", func(t *testing.T) {
		store := newFakeRunningStore(scb, sharedPool)
		close(store.firstUnblock)

		respChan := make(chan *gnmi.SubscribeResponse, 1)
		target := &fakeSyncTarget{
			respChan: respChan,
			errChan:  make(chan error, 1),
		}

		os, err := NewOnceSync(ctx, target, c, store, scb, sharedPool)
		if err != nil {
			t.Fatalf("NewOnceSync: %v", err)
		}
		defer func() { _ = os.Stop() }()

		if err := os.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}

		respChan <- interfaceDescriptionNotif("ethernet-1/1", "no sync response")
		close(respChan)
		time.Sleep(300 * time.Millisecond)

		if store.isSynced(syncName) {
			t.Error("runningStore reports Synced without SyncResponse")
		}
	})

	t.Run("invalid path fails at construction", func(t *testing.T) {
		bad := &config.SyncProtocol{
			Name:  syncName,
			Mode:  "once",
			Paths: []string{"/interface[name=foo"},
		}
		_, err := NewOnceSync(ctx, &fakeSyncTarget{}, bad, newFakeRunningStore(scb, sharedPool), scb, sharedPool)
		if err == nil {
			t.Fatal("NewOnceSync: expected error for invalid path")
		}
	})
}

func TestOnceSync_SkipsIntervalTickWhileCycleInFlight(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sc, schemaConf, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatalf("init schema: %v", err)
	}
	scb := schemaClientPkg.NewSchemaClientBound(schemaConf, sc)
	sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

	store := newFakeRunningStore(scb, sharedPool)
	// Block ApplyToRunning so the first cycle stays in flight past the tick.
	// Do not close firstUnblock until after we observe subscribe count.

	respChan := make(chan *gnmi.SubscribeResponse, 10)
	target := newCountingFakeSyncTarget(respChan, make(chan error, 1))

	const syncName = "once-inflight"
	c := &config.SyncProtocol{
		Name:     syncName,
		Mode:     "once",
		Paths:    []string{"/interface"},
		Interval: 50 * time.Millisecond,
	}

	os, err := NewOnceSync(ctx, target, c, store, scb, sharedPool)
	if err != nil {
		t.Fatalf("NewOnceSync: %v", err)
	}
	defer func() { _ = os.Stop() }()

	if err := os.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-target.subscribes:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial subscribe")
	}

	respChan <- syncRespMsg()

	select {
	case <-store.firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ApplyToRunning to start (cycle should be in flight)")
	}

	time.Sleep(200 * time.Millisecond)
	subscribeCount := 1
	for {
		select {
		case <-target.subscribes:
			subscribeCount++
		default:
			goto doneDrain
		}
	}
doneDrain:

	if subscribeCount != 1 {
		t.Errorf("Subscribe called %d times while first cycle blocked; want 1 (interval tick skipped)", subscribeCount)
	}

	close(store.firstUnblock)
	waitUntilSynced(t, store, syncName)
}

func waitUntilSynced(t *testing.T, store *fakeRunningStore, syncName string) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if store.isSynced(syncName) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("runningStore.MarkSynced(%q) was not called after a successful ONCE cycle", syncName)
}
