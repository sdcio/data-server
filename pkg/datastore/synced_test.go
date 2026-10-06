package datastore

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/sdcio/data-server/mocks/mockcacheclient"
	"github.com/sdcio/data-server/mocks/mocktarget"
	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/datastore/types"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	"github.com/sdcio/sdc-protos/tree_persist"
	"go.uber.org/mock/gomock"
)

// TestSynced_ZeroConfiguredSyncs_TriviallySynced verifies that a datastore
// with no configured syncs (nil Sync, or a Sync with an empty Config) is
// immediately Synced, and never needs MarkSynced.
func TestSynced_ZeroConfiguredSyncs_TriviallySynced(t *testing.T) {
	tests := []struct {
		name string
		sync *config.Sync
	}{
		{name: "nil Sync", sync: nil},
		{name: "empty Sync.Config", sync: &config.Sync{Config: nil}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &Datastore{
				config: &config.DatastoreConfig{Name: "test-ds", Sync: tt.sync},
			}
			if !ds.Synced() {
				t.Errorf("Synced() = false, want true for %s", tt.name)
			}
		})
	}
}

// TestSynced_LatchesPermanentlyOnceAllNamesMarked verifies that Synced only
// becomes true once every configured sync name has been marked, and that it
// latches permanently thereafter (simulating a disconnect/reconnect, which
// never calls AddSyncs / MarkSynced again).
func TestSynced_LatchesPermanentlyOnceAllNamesMarked(t *testing.T) {
	ds := &Datastore{
		config: &config.DatastoreConfig{
			Name: "test-ds",
			Sync: &config.Sync{Config: []*config.SyncProtocol{
				{Name: "s1"},
				{Name: "s2"},
			}},
		},
	}

	if ds.Synced() {
		t.Fatal("Synced() = true before any MarkSynced call, want false")
	}

	ds.MarkSynced("s1")
	if ds.Synced() {
		t.Fatal("Synced() = true after only one of two syncs marked, want false")
	}

	ds.MarkSynced("s2")
	if !ds.Synced() {
		t.Fatal("Synced() = false after all syncs marked, want true")
	}

	// Idempotent: repeated / redundant calls are no-ops and the latch never
	// reverts (this is the "survives reconnect" invariant, since a
	// reconnect never calls MarkSynced again but Synced must stay true).
	ds.MarkSynced("s1")
	ds.MarkSynced("unknown-sync-name")
	if !ds.Synced() {
		t.Error("Synced() reverted to false after redundant MarkSynced calls, want true")
	}
}

// TestSynced_LazyInitFromDirectlyConstructedDatastore verifies that Synced()
// / MarkSynced() work correctly even when a Datastore is constructed as a
// bare struct literal (as many existing tests do), without going through
// datastore.New(), by lazily seeding from config on first use.
func TestSynced_LazyInitFromDirectlyConstructedDatastore(t *testing.T) {
	ds := &Datastore{
		config: &config.DatastoreConfig{
			Name: "test-ds",
			Sync: &config.Sync{Config: []*config.SyncProtocol{{Name: "only-sync"}}},
		},
	}

	if ds.Synced() {
		t.Fatal("Synced() = true before MarkSynced, want false")
	}
	ds.MarkSynced("only-sync")
	if !ds.Synced() {
		t.Fatal("Synced() = false after MarkSynced, want true")
	}
}

// TestSynced_ConcurrentMarkSynced verifies MarkSynced is safe under
// concurrent calls and still latches correctly.
func TestSynced_ConcurrentMarkSynced(t *testing.T) {
	names := []string{"s1", "s2", "s3", "s4"}
	syncCfg := make([]*config.SyncProtocol, 0, len(names))
	for _, n := range names {
		syncCfg = append(syncCfg, &config.SyncProtocol{Name: n})
	}
	ds := &Datastore{
		config: &config.DatastoreConfig{
			Name: "test-ds",
			Sync: &config.Sync{Config: syncCfg},
		},
	}

	var wg sync.WaitGroup
	for _, n := range names {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			ds.MarkSynced(name)
		}(n)
	}
	wg.Wait()

	if !ds.Synced() {
		t.Error("Synced() = false after all syncs marked concurrently, want true")
	}
}

// newGatedTestDatastore builds a minimal Datastore suitable for exercising
// the TransactionSet Synced gate: one configured (not-yet-synced) sync name,
// and enough scaffolding (dmutex, transactionManager) for TransactionSet to
// run up to the gate.
func newGatedTestDatastore(t *testing.T, ctrl *gomock.Controller) *Datastore {
	t.Helper()

	ctx := context.Background()
	sc, schemaConf, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatalf("init schema: %v", err)
	}
	scb := schemaClient.NewSchemaClientBound(schemaConf, sc)
	taskPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	tc := tree.NewTreeContext(scb, taskPool)
	syncTreeRoot, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatalf("new tree root: %v", err)
	}

	ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
	// LoadAllButRunningIntents (called by both replaceIntent and
	// lowlevelTransactionSet) always calls IntentGetAll; give it a default
	// empty-result expectation so tests that don't care about intent
	// content don't hang on an unmet mock expectation.
	ccb.EXPECT().
		IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
			close(intentChan)
			close(errChan)
		}).AnyTimes()

	ds := &Datastore{
		config: &config.DatastoreConfig{
			Name:       "test-ds",
			Validation: config.NewValidationConfig(),
			Sync: &config.Sync{Config: []*config.SyncProtocol{
				{Name: "s1"},
			}},
		},
		dmutex:        &sync.Mutex{},
		cacheClient:   ccb,
		sbi:           mocktarget.NewMockTarget(ctrl),
		schemaClient:  scb,
		taskPool:      taskPool,
		syncTree:      syncTreeRoot,
		syncTreeMutex: &sync.RWMutex{},
	}
	ds.transactionManager = types.NewTransactionManager(NewDatastoreRollbackAdapter(ds))
	return ds
}

// TestTransactionSet_MergePhase_GatedOnSynced verifies that TransactionSet
// (no replace, ordinary intents — the "merge phase") rejects with
// ErrNotSynced before Running has completed its first sync, for both a
// normal Set and a dryRun, and does not touch the cache/SBI mocks while
// gated.
func TestTransactionSet_MergePhase_GatedOnSynced(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run("dryRun="+boolStr(dryRun), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			ds := newGatedTestDatastore(t, ctrl)

			ti := types.NewTransactionIntent("intent1", 10)
			resp, err := ds.TransactionSet(context.Background(), "txn-gate-merge-"+boolStr(dryRun),
				[]*types.TransactionIntent{ti}, nil, 10*time.Second, dryRun)

			if !errors.Is(err, ErrNotSynced) {
				t.Fatalf("TransactionSet() error = %v, want ErrNotSynced", err)
			}
			if resp != nil {
				t.Errorf("TransactionSet() response = %v, want nil", resp)
			}
		})
	}
}

// TestTransactionSet_ReplacePhase_GatedOnSynced verifies that a replace
// transaction (dryRun and non-dryRun) rejects with ErrNotSynced before
// Running has completed its first sync, and never reaches replaceIntent's
// cache access.
func TestTransactionSet_ReplacePhase_GatedOnSynced(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run("dryRun="+boolStr(dryRun), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			ds := newGatedTestDatastore(t, ctrl)
			// No EXPECT() set on cacheClient.IntentGet: if replaceIntent were
			// reached despite the gate, gomock would fail the test on the
			// unexpected call.

			replace := types.NewTransactionIntent("replace-intent", 0)
			resp, err := ds.TransactionSet(context.Background(), "txn-gate-replace-"+boolStr(dryRun),
				nil, replace, 10*time.Second, dryRun)

			if !errors.Is(err, ErrNotSynced) {
				t.Fatalf("TransactionSet() error = %v, want ErrNotSynced", err)
			}
			if resp != nil {
				t.Errorf("TransactionSet() response = %v, want nil", resp)
			}
		})
	}
}

// TestTransactionSet_NoOp_NeverGated verifies that a no-op transaction (no
// intents, no replace) always succeeds regardless of Synced state.
func TestTransactionSet_NoOp_NeverGated(t *testing.T) {
	ctrl := gomock.NewController(t)
	ds := newGatedTestDatastore(t, ctrl)

	resp, err := ds.TransactionSet(context.Background(), "txn-noop", nil, nil, 10*time.Second, false)
	if err != nil {
		t.Fatalf("TransactionSet() error = %v, want nil for a no-op transaction", err)
	}
	if resp == nil {
		t.Fatal("TransactionSet() response = nil, want non-nil")
	}
}

// TestTransactionSet_MergePhase_SucceedsAfterSynced verifies that once every
// configured sync is marked, the merge-phase gate no longer rejects the
// transaction (it proceeds to lowlevelTransactionSet, which may still fail
// for unrelated reasons, but must not fail with ErrNotSynced).
func TestTransactionSet_MergePhase_SucceedsAfterSynced(t *testing.T) {
	ctrl := gomock.NewController(t)
	ds := newGatedTestDatastore(t, ctrl)
	ds.MarkSynced("s1")

	// Let lowlevelTransactionSet fail for an unrelated (mock-scaffolding)
	// reason; we only assert it is not ErrNotSynced, proving the gate let it
	// through.
	wantErr := errors.New("unrelated mock-scaffolding failure")
	ds.sbi.(*mocktarget.MockTarget).EXPECT().
		Set(gomock.Any(), gomock.Any()).
		Return(nil, wantErr).AnyTimes()

	ti := types.NewTransactionIntent("intent1", 10)
	_, err := ds.TransactionSet(context.Background(), "txn-post-sync", []*types.TransactionIntent{ti}, nil, 10*time.Second, false)
	if errors.Is(err, ErrNotSynced) {
		t.Fatalf("TransactionSet() error = %v, want anything but ErrNotSynced once Synced", err)
	}
}

// TestTransactionRollback_NotGatedOnSynced verifies that
// DatastoreRollbackAdapter.TransactionRollback (used exclusively for rollback
// of an already-gated transaction) bypasses the Synced gate entirely, since
// it calls lowlevelTransactionSet directly rather than TransactionSet.
func TestTransactionRollback_NotGatedOnSynced(t *testing.T) {
	ctrl := gomock.NewController(t)
	ds := newGatedTestDatastore(t, ctrl)
	// Deliberately not synced.
	if ds.Synced() {
		t.Fatal("test setup: datastore unexpectedly already Synced")
	}

	rollbackTx := types.NewTransaction("txn-rollback", ds.transactionManager)

	adapter := NewDatastoreRollbackAdapter(ds)
	_, err := adapter.TransactionRollback(context.Background(), rollbackTx, true)
	if errors.Is(err, ErrNotSynced) {
		t.Fatalf("TransactionRollback() error = %v, want anything but ErrNotSynced (rollback must not be gated)", err)
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}