package gnmi

import (
	"context"
	"runtime"
	"testing"
	"time"

	schemaClientPkg "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// fakeGetTarget implements GetTarget, returning a fixed response (or error)
// for every Get call and counting invocations.
type fakeGetTarget struct {
	resp *sdcpb.GetDataResponse
	err  error
	gets chan struct{}
}

func newFakeGetTarget(resp *sdcpb.GetDataResponse, err error) *fakeGetTarget {
	return &fakeGetTarget{resp: resp, err: err, gets: make(chan struct{}, 10)}
}

func (f *fakeGetTarget) Get(_ context.Context, _ *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
	select {
	case f.gets <- struct{}{}:
	default:
	}
	return f.resp, f.err
}

// TestGetSync_MarksSyncedAfterFirstSuccessfulCycle verifies that GetSync
// calls runningStore.MarkSynced for its own name only after
// internalGetSync's ApplyToRunning succeeds, and never while every attempt
// fails.
func TestGetSync_MarksSyncedAfterFirstSuccessfulCycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sc, schemaConf, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatalf("init schema: %v", err)
	}
	scb := schemaClientPkg.NewSchemaClientBound(schemaConf, sc)
	sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

	const syncName = "get-sync"
	c := &config.SyncProtocol{
		Name:     syncName,
		Paths:    []string{"/interface"},
		Interval: time.Hour, // long enough that the periodic ticker doesn't fire during the test
	}

	t.Run("failing Get never marks Synced", func(t *testing.T) {
		store := newFakeRunningStore(scb, sharedPool)
		close(store.firstUnblock)

		target := newFakeGetTarget(nil, context.DeadlineExceeded)
		s, err := NewGetSync(ctx, target, c, store, scb)
		if err != nil {
			t.Fatalf("NewGetSync: %v", err)
		}
		defer func() { _ = s.Stop() }()

		if err := s.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}

		select {
		case <-target.gets:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for the (failing) initial Get")
		}
		// give internalGetSync a moment to finish processing the error path.
		time.Sleep(100 * time.Millisecond)

		if store.isSynced(syncName) {
			t.Error("runningStore reports Synced after a failed sync cycle, want not-Synced")
		}
	})

	t.Run("successful Get marks Synced", func(t *testing.T) {
		store := newFakeRunningStore(scb, sharedPool)
		close(store.firstUnblock)

		resp := &sdcpb.GetDataResponse{
			Notification: []*sdcpb.Notification{
				{
					Timestamp: time.Now().UnixNano(),
					Update: []*sdcpb.Update{
						{
							Path: &sdcpb.Path{
								Elem: []*sdcpb.PathElem{
									{Name: "interface", Key: map[string]string{"name": "ethernet-1/1"}},
									{Name: "description"},
								},
							},
							Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "get-sync description"}},
						},
					},
				},
			},
		}
		target := newFakeGetTarget(resp, nil)
		s, err := NewGetSync(ctx, target, c, store, scb)
		if err != nil {
			t.Fatalf("NewGetSync: %v", err)
		}
		defer func() { _ = s.Stop() }()

		if err := s.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}

		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if store.isSynced(syncName) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		if !store.isSynced(syncName) {
			t.Errorf("runningStore.MarkSynced(%q) was not called after a successful sync cycle", syncName)
		}
	})
}
