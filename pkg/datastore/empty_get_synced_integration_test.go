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
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi"
	"github.com/sdcio/data-server/pkg/datastore/types"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	"github.com/sdcio/sdc-protos/tree_persist"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// emptyGetTarget returns a successful gNMI GET with no notifications (empty
// snapshot under configured sync paths).
type emptyGetTarget struct{}

func (emptyGetTarget) Get(_ context.Context, _ *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
	return &sdcpb.GetDataResponse{}, nil
}

// newGetSyncIntegrationDatastore is a real Datastore used as RunningStore for
// a single configured gNMI GET sync entry.
func newGetSyncIntegrationDatastore(t *testing.T, ctrl *gomock.Controller, syncName string) *Datastore {
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
	ccb.EXPECT().
		IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
			close(intentChan)
			close(errChan)
		}).AnyTimes()
	ccb.EXPECT().
		IntentDelete(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	mockSBI := mocktarget.NewMockTarget(ctrl)
	mockSBI.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).AnyTimes()

	ds := &Datastore{
		config: &config.DatastoreConfig{
			Name:       "test-ds",
			Validation: config.NewValidationConfig(),
			Sync: &config.Sync{Config: []*config.SyncProtocol{{
				Name:     syncName,
				Mode:     "get",
				Paths:    []string{"/interface"},
				Interval: time.Hour,
			}}},
		},
		dmutex:        &sync.Mutex{},
		cacheClient:   ccb,
		sbi:           mockSBI,
		schemaClient:  scb,
		taskPool:      taskPool,
		syncTree:      syncTreeRoot,
		syncTreeMutex: &sync.RWMutex{},
	}
	ds.transactionManager = types.NewTransactionManager(NewDatastoreRollbackAdapter(ds))
	return ds
}

// TestEmptyGetSyncCycle_EndToEnd verifies that an empty successful GET sync
// against a real Datastore (as RunningStore) latches Synced and clears the
// TransactionSet gate.
func TestEmptyGetSyncCycle_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	const syncName = "get-sync"
	ctrl := gomock.NewController(t)
	ds := newGetSyncIntegrationDatastore(t, ctrl, syncName)

	syncProto := ds.config.Sync.Config[0]

	if ds.Synced() {
		t.Fatal("Synced() = true before any sync cycle, want false")
	}

	ti := types.NewTransactionIntent("intent1", 10)
	_, err := ds.TransactionSet(ctx, "txn-pre-sync", []*types.TransactionIntent{ti}, nil, 10*time.Second, false)
	if !errors.Is(err, ErrNotSynced) {
		t.Fatalf("TransactionSet() before sync error = %v, want ErrNotSynced", err)
	}

	getSync, err := gnmi.NewGetSync(ctx, emptyGetTarget{}, syncProto, ds, ds.schemaClient)
	if err != nil {
		t.Fatalf("NewGetSync: %v", err)
	}
	defer func() { _ = getSync.Stop() }()

	if err := getSync.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if ds.Synced() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ds.Synced() {
		t.Fatal("Synced() = false after empty GET sync cycle, want true")
	}

	_, err = ds.TransactionSet(ctx, "txn-post-sync", []*types.TransactionIntent{ti}, nil, 10*time.Second, false)
	if errors.Is(err, ErrNotSynced) {
		t.Fatalf("TransactionSet() after empty GET sync error = %v, want anything but ErrNotSynced", err)
	}
}
