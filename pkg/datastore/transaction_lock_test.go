package datastore

import (
	"context"
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
	"github.com/sdcio/data-server/pkg/tree/consts"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	"go.uber.org/mock/gomock"
)

// newLockTestDatastore returns a Datastore backed by strict mocks: any unexpected
// cache write or southbound Set fails the test.
func newLockTestDatastore(t *testing.T) (*Datastore, *mocktarget.MockTarget, *mockcacheclient.MockCacheClientBound) {
	t.Helper()
	ctx := context.Background()
	ctrl := gomock.NewController(t)

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
	ccb.EXPECT().
		IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
			close(intentChan)
			close(errChan)
		}).AnyTimes()
	ccb.EXPECT().IntentGet(gomock.Any(), consts.RunningIntentName).
		Return(&tree_persist.Intent{IntentName: consts.RunningIntentName}, nil).AnyTimes()

	sbi := mocktarget.NewMockTarget(ctrl)

	pl := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	syncTree, err := tree.NewTreeRoot(ctx, tree.NewTreeContext(scb, pl))
	if err != nil {
		t.Fatal(err)
	}
	if err := syncTree.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}

	ds := &Datastore{
		config:        &config.DatastoreConfig{Validation: config.NewValidationConfig(), Name: "test-ds"},
		syncTreeMutex: &sync.RWMutex{},
		syncTree:      syncTree,
		taskPool:      pl,
		cacheClient:   ccb,
		sbi:           sbi,
		dmutex:        &sync.Mutex{},
		schemaClient:  scb,
	}
	ds.transactionManager = types.NewTransactionManager(NewDatastoreRollbackAdapter(ds), types.WithAutoRollbackLock(ds.dmutex))
	return ds, sbi, ccb
}

func TestTransactionSet_DryRunNoOpDoesNotPanic(t *testing.T) {
	ds, _, _ := newLockTestDatastore(t)

	rsp, err := ds.TransactionSet(context.Background(), "x", nil, nil, time.Minute, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rsp.GetWarnings()) == 0 {
		t.Error("expected the no intents warning")
	}
	if err := ds.TransactionConfirm(context.Background(), "x"); err == nil {
		t.Error("dry-run must not register a transaction")
	}
}

// A dry-run replace may validate but must neither reach the device nor change the running state.
func TestTransactionSet_DryRunReplaceDoesNotTouchDevice(t *testing.T) {
	ctx := context.Background()
	ds, _, _ := newLockTestDatastore(t) // no Set / IntentModify expectations registered

	replace := types.NewTransactionIntent(consts.ReplaceIntentName, consts.ReplaceValuesPrio)
	updates, err := treetypes.ExpandAndConvertIntent(ctx, ds.schemaClient, consts.ReplaceIntentName, consts.ReplaceValuesPrio, []*sdcpb.Update{{
		Path: &sdcpb.Path{},
		Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{
			JsonVal: []byte(`{"interface":[{"name":"ethernet-1/1","description":"replaced"}]}`),
		}},
	}}, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	replace.AddUpdates(updates)

	if _, err := ds.TransactionSet(ctx, "x", nil, replace, time.Minute, true); err != nil {
		t.Fatal(err)
	}
}

// Confirm only touches the transaction manager and must work while another set-path holder owns dmutex.
func TestTransactionConfirm_NotBlockedByDmutex(t *testing.T) {
	ctx := context.Background()
	ds, _, _ := newLockTestDatastore(t)

	if _, err := ds.TransactionSet(ctx, "x", nil, nil, time.Minute, false); err != nil {
		t.Fatal(err)
	}

	ds.dmutex.Lock()
	defer ds.dmutex.Unlock()
	if err := ds.TransactionConfirm(ctx, "x"); err != nil {
		t.Fatalf("confirm refused while dmutex is held: %v", err)
	}
}

// A no-op transaction is registered and must be released by the auto rollback timer if never confirmed.
func TestTransactionSet_NoOpIsReleasedOnTimeout(t *testing.T) {
	ctx := context.Background()
	ds, _, _ := newLockTestDatastore(t)

	if _, err := ds.TransactionSet(ctx, "x", nil, nil, 20*time.Millisecond, false); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.TransactionSet(ctx, "y", nil, nil, time.Minute, false); err == nil {
		t.Fatal("second transaction must be refused while the first is unconfirmed")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := ds.TransactionSet(ctx, "z", nil, nil, time.Minute, false)
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no-op transaction was never released: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
